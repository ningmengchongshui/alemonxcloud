package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

type deploymentTask struct {
	ID         string `json:"id"`
	InstanceID string `json:"instanceId"`
	Action     string `json:"action"`
	Attempt    int    `json:"attempt"`
}

var taskChannel *amqp091.Channel

func initTaskQueue() error {
	connection, err := amqp091.Dial(env("RABBITMQ_URL", ""))
	if err != nil {
		return err
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return err
	}
	if _, err := channel.QueueDeclare("xcloud.deployment", true, false, false, false, nil); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return err
	}
	taskChannel = channel
	return nil
}

func enqueueTask(ctx context.Context, task deploymentTask) error {
	if taskChannel == nil {
		return fmt.Errorf("任务队列不可用")
	}
	body, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return taskChannel.PublishWithContext(ctx, "", "xcloud.deployment", false, false, amqp091.Publishing{ContentType: "application/json", DeliveryMode: amqp091.Persistent, Timestamp: time.Now(), Body: body})
}

func consumeTasks() {
	if taskChannel == nil {
		return
	}
	deliveries, err := taskChannel.Consume("xcloud.deployment", "xcloud-server", false, false, false, false, nil)
	if err != nil {
		log.Printf("start task consumer: %v", err)
		return
	}
	go func() {
		for delivery := range deliveries {
			var task deploymentTask
			if err := json.Unmarshal(delivery.Body, &task); err != nil {
				_ = delivery.Nack(false, false)
				continue
			}
			log.Printf("received deployment task %s for %s", task.Action, task.InstanceID)
			_ = delivery.Ack(false)
		}
	}()
}
