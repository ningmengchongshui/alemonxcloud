package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

type deploymentTask struct {
	ID         string `json:"id"`
	InstanceID string `json:"instanceId"`
	Action     string `json:"action"`
	Attempt    int    `json:"attempt"`
}

var queueMu sync.RWMutex
var taskChannel *amqp091.Channel
var taskConnection *amqp091.Connection

func initTaskQueue() error { return connectTaskQueue() }
func queueAvailable() bool {
	queueMu.RLock()
	defer queueMu.RUnlock()
	return taskConnection != nil && !taskConnection.IsClosed() && taskChannel != nil && !taskChannel.IsClosed()
}

func connectTaskQueue() error {
	queueMu.Lock()
	defer queueMu.Unlock()
	if taskConnection != nil && !taskConnection.IsClosed() && taskChannel != nil && !taskChannel.IsClosed() {
		return nil
	}
	connection, err := amqp091.Dial(env("RABBITMQ_URL", ""))
	if err != nil {
		return err
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return err
	}
	if err := declareTaskTopology(channel); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return err
	}
	taskConnection, taskChannel = connection, channel
	go watchTaskConnection(connection)
	go consumeTaskChannel(channel)
	return nil
}
func declareTaskTopology(channel *amqp091.Channel) error {
	if err := channel.ExchangeDeclare("xcloud.deadletter", "direct", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := channel.QueueDeclare("xcloud.deployment.dead", true, false, false, false, nil); err != nil {
		return err
	}
	if err := channel.QueueBind("xcloud.deployment.dead", "failed", "xcloud.deadletter", false, nil); err != nil {
		return err
	}
	if _, err := channel.QueueDeclare("xcloud.deployment", true, false, false, false, amqp091.Table{"x-dead-letter-exchange": "xcloud.deadletter", "x-dead-letter-routing-key": "failed"}); err != nil {
		return err
	}
	return nil
}
func watchTaskConnection(connection *amqp091.Connection) {
	<-connection.NotifyClose(make(chan *amqp091.Error, 1))
	queueMu.Lock()
	if taskConnection == connection {
		taskConnection = nil
		taskChannel = nil
	}
	queueMu.Unlock()
	for delay := time.Second; ; delay = minDuration(delay*2, 30*time.Second) {
		time.Sleep(delay)
		if err := connectTaskQueue(); err == nil {
			recoverPendingTasks()
			return
		} else {
			log.Printf("RabbitMQ reconnect: %v", err)
		}
	}
}
func enqueueTask(ctx context.Context, task deploymentTask) error {
	if err := connectTaskQueue(); err != nil {
		return fmt.Errorf("任务队列不可用: %w", err)
	}
	body, err := json.Marshal(task)
	if err != nil {
		return err
	}
	queueMu.RLock()
	connection := taskConnection
	queueMu.RUnlock()
	if connection == nil || connection.IsClosed() {
		return fmt.Errorf("任务队列不可用")
	}
	publisher, err := connection.Channel()
	if err != nil {
		return err
	}
	defer publisher.Close()
	if err = publisher.Confirm(false); err != nil {
		return err
	}
	confirms := publisher.NotifyPublish(make(chan amqp091.Confirmation, 1))
	if err = publisher.PublishWithContext(ctx, "", "xcloud.deployment", false, false, amqp091.Publishing{ContentType: "application/json", DeliveryMode: amqp091.Persistent, Timestamp: time.Now(), Body: body}); err != nil {
		return err
	}
	select {
	case confirmation := <-confirms:
		if !confirmation.Ack {
			return fmt.Errorf("RabbitMQ 未确认投递")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
		return fmt.Errorf("等待 RabbitMQ 确认超时")
	}
}

func consumeTasks() {} // consumer is started whenever a connection is established.
func consumeTaskChannel(channel *amqp091.Channel) {
	if err := channel.Qos(1, 0, false); err != nil {
		log.Printf("configure task consumer: %v", err)
		return
	}
	deliveries, err := channel.Consume("xcloud.deployment", "xcloud-server", false, false, false, false, nil)
	if err != nil {
		log.Printf("start task consumer: %v", err)
		return
	}
	for delivery := range deliveries {
		processDelivery(delivery)
	}
}
func processDelivery(delivery amqp091.Delivery) {
	var message deploymentTask
	if err := json.Unmarshal(delivery.Body, &message); err != nil {
		_ = delivery.Nack(false, false)
		return
	}
	if instanceDB == nil {
		_ = delivery.Nack(false, true)
		return
	}
	task, err := loadTask(context.Background(), message.ID)
	if err != nil {
		_ = delivery.Ack(false)
		return
	}
	claimed, err := claimTask(context.Background(), task.ID)
	if err != nil || !claimed {
		_ = delivery.Ack(false)
		return
	}
	task.Attempts++
	task.WorkerID = taskWorkerID()
	err = executeTask(context.Background(), task)
	finished, finishErr := finishTask(context.Background(), task, err)
	if finishErr != nil {
		log.Printf("finish task %s: %v", task.ID, finishErr)
	}
	if !finished {
		// The lease was recovered or ownership changed while the Agent call was
		// in flight. The durable replacement attempt owns the final outcome.
		_ = delivery.Ack(false)
		return
	}
	if err != nil {
		failDeployment(context.Background(), task, err)
		log.Printf("task %s %s failed (attempt %d): %v", task.ID, task.Action, task.Attempts, err)
		if task.Attempts >= 3 {
			_ = delivery.Nack(false, false)
		} else {
			_ = delivery.Ack(false)
		}
		return
	}
	_ = writeAudit(context.Background(), "system", "task.succeeded", "task", task.ID, map[string]any{"action": task.Action, "instanceId": task.InstanceID})
	_ = delivery.Ack(false)
}
func recoverPendingTasks() {
	if instanceDB == nil || !queueAvailable() {
		return
	}
	items, err := pendingTasks(context.Background(), 100)
	if err != nil {
		log.Printf("recover pending tasks: %v", err)
		return
	}
	for _, task := range items {
		if err := enqueuePersistedTask(context.Background(), task); err != nil {
			log.Printf("republish task %s: %v", task.ID, err)
		}
	}
}

// recoverExpiredTaskLeases returns work claimed by a crashed consumer to the
// durable queue. The conditional update guarantees a live consumer cannot be
// reclaimed before its five minute lease expires.
func recoverExpiredTaskLeases(ctx context.Context) {
	if instanceDB == nil {
		return
	}
	rows, err := instanceDB.QueryContext(ctx, `SELECT `+taskSelectFields+` FROM xcloud_tasks WHERE status=? AND claim_expires_at IS NOT NULL AND claim_expires_at<=NOW() ORDER BY claim_expires_at LIMIT 100`, taskRunning)
	if err != nil {
		log.Printf("load expired task leases: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var task controlTask
		if err := scanControlTask(rows, &task); err != nil {
			continue
		}
		result, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks SET status=?,run_after=NOW(),claimed_at=NULL,claim_expires_at=NULL,worker_id=NULL,updated_at=NOW() WHERE id=? AND status=? AND claim_expires_at<=NOW()`, taskPending, task.ID, taskRunning)
		if err != nil {
			log.Printf("recover task lease %s: %v", task.ID, err)
			continue
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			continue
		}
		appendTaskEvent(ctx, task.ID, "lease_recovered", "消费者租约已过期，任务重新排队")
		_ = writeAudit(ctx, "system", "task.lease_recovered", "task", task.ID, map[string]any{"instanceId": task.InstanceID})
		if queueAvailable() {
			if err := enqueuePersistedTask(ctx, task); err != nil {
				log.Printf("republish recovered task %s: %v", task.ID, err)
			}
		}
	}
}
func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
