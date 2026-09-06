package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
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
	workers := taskWorkerConcurrency()
	if err := channel.Qos(workers, 0, false); err != nil {
		log.Printf("configure task consumer: %v", err)
		return
	}
	deliveries, err := channel.Consume("xcloud.deployment", "xcloud-server", false, false, false, false, nil)
	if err != nil {
		log.Printf("start task consumer: %v", err)
		return
	}
	sem := make(chan struct{}, workers)
	for delivery := range deliveries {
		sem <- struct{}{}
		go func(delivery amqp091.Delivery) {
			defer func() { <-sem }()
			processDelivery(delivery)
		}(delivery)
	}
}

func taskWorkerConcurrency() int {
	const fallback = 4
	value, err := strconv.Atoi(strings.TrimSpace(env("XCLOUD_TASK_WORKERS", strconv.Itoa(fallback))))
	if err != nil || value < 1 || value > 16 {
		return fallback
	}
	return value
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
	claimed, err := claimTask(context.Background(), task)
	if err != nil || !claimed {
		_ = delivery.Ack(false)
		return
	}
	task.Attempts++
	task.WorkerID = taskWorkerID()
	// Reload the execution token written by the atomic claim before any Agent
	// call. A stale worker can never complete or release a newer execution.
	if task, err = loadTask(context.Background(), task.ID); err != nil {
		_ = delivery.Ack(false)
		return
	}
	leaseStop := make(chan struct{})
	go renewTaskLease(task, leaseStop)
	err = executeTask(context.Background(), task)
	close(leaseStop)
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

func renewTaskLease(task controlTask, stop <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			result, err := instanceDB.ExecContext(context.Background(), `UPDATE xcloud_tasks SET heartbeat_at=NOW(),claim_expires_at=DATE_ADD(NOW(), INTERVAL 5 MINUTE),updated_at=NOW() WHERE id=? AND status=? AND worker_id=? AND execution_token=? AND claim_expires_at>NOW()`, task.ID, taskRunning, task.WorkerID, task.ExecutionToken)
			if err != nil {
				log.Printf("renew task lease %s: %v", task.ID, err)
				appendTaskEvent(context.Background(), task.ID, "lease_renew_failed", truncateError(err.Error()))
				continue
			}
			if n, _ := result.RowsAffected(); n != 1 {
				appendTaskEvent(context.Background(), task.ID, "lease_renew_failed", "任务已失去执行租约")
				return
			}
			if lifecycleTask(task.Action) {
				_, err = instanceDB.ExecContext(context.Background(), `UPDATE xcloud_instances SET active_task_expires_at=DATE_ADD(NOW(), INTERVAL 5 MINUTE) WHERE id=? AND active_task_id=? AND active_task_token=?`, task.InstanceID, task.ID, task.ExecutionToken)
				if err != nil {
					log.Printf("renew instance lock %s: %v", task.InstanceID, err)
					appendTaskEvent(context.Background(), task.ID, "lease_renew_failed", truncateError(err.Error()))
				}
			}
		}
	}
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
		next, detail := taskPending, "消费者租约已过期，任务重新排队"
		if dangerousRecoveredTask(task.Action) || !safeRecoveryState(ctx, task) {
			next, detail = taskReview, "历史生命周期任务已隔离，等待管理员复核"
		}
		result, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks SET status=?,run_after=NOW(),claimed_at=NULL,claim_expires_at=NULL,worker_id=NULL,execution_token=NULL,recovery_count=recovery_count+1,last_error=?,updated_at=NOW() WHERE id=? AND status=? AND claim_expires_at<=NOW()`, next, detail, task.ID, taskRunning)
		if err != nil {
			log.Printf("recover task lease %s: %v", task.ID, err)
			continue
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			continue
		}
		event := "lease_recovered"
		if next == taskReview {
			event = "recovery_quarantined"
		}
		appendTaskEvent(ctx, task.ID, event, detail)
		_ = writeAudit(ctx, "system", "task."+event, "task", task.ID, map[string]any{"instanceId": task.InstanceID, "action": task.Action})
		if next == taskPending && queueAvailable() {
			if err := enqueuePersistedTask(ctx, task); err != nil {
				log.Printf("republish recovered task %s: %v", task.ID, err)
			}
		}
	}
}

// quarantineDangerousFailedTasks cleans up failures created before lifecycle
// fencing was introduced. A failed update/restart/destroy has an unknown
// remote state, so treating it as a normal retry is unsafe. Keep the record
// and its error, but require an explicit review before any replay.
func quarantineDangerousFailedTasks(ctx context.Context) {
	if instanceDB == nil {
		return
	}
	rows, err := instanceDB.QueryContext(ctx, `SELECT id,instance_id,action FROM xcloud_tasks
		WHERE status=? AND action IN ('stop','update','restart','reinstall','destroy','purge','retry-deploy')
		ORDER BY updated_at ASC LIMIT 100`, taskFailed)
	if err != nil {
		log.Printf("load failed lifecycle tasks for quarantine: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, instanceID, action string
		if err := rows.Scan(&id, &instanceID, &action); err != nil {
			continue
		}
		result, err := instanceDB.ExecContext(ctx, `UPDATE xcloud_tasks
			SET status=?,last_error=CONCAT(COALESCE(last_error,''), '\n已自动隔离：危险生命周期任务需人工复核后才能恢复'),updated_at=NOW()
			WHERE id=? AND status=?`, taskReview, id, taskFailed)
		if err != nil {
			log.Printf("quarantine failed lifecycle task %s: %v", id, err)
			continue
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			continue
		}
		appendTaskEvent(ctx, id, "failed_task_quarantined", "危险生命周期任务已自动转入人工复核，未重放容器操作")
		_ = writeAudit(ctx, "system", "task.failed_quarantined", "task", id, map[string]any{"instanceId": instanceID, "action": action})
	}
}

func dangerousRecoveredTask(action string) bool {
	return action == "stop" || action == "update" || action == "restart" || action == "reinstall" || action == "destroy" || action == "purge" || action == "retry-deploy" || action == "resize"
}
func safeRecoveryState(ctx context.Context, task controlTask) bool {
	if task.Action != "create" && task.Action != "start" {
		return task.Action == "bandwidth"
	}
	var status string
	if err := instanceDB.QueryRowContext(ctx, `SELECT status FROM xcloud_instances WHERE id=?`, task.InstanceID).Scan(&status); err != nil {
		return false
	}
	return (task.Action == "create" && status == "deploying") || (task.Action == "start" && (status == "stopped" || status == "destroy_scheduled"))
}
func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
