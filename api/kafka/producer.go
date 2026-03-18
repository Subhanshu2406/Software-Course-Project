// Package kafka provides a Kafka producer to publish transactions for asynchronous processing.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/segmentio/kafka-go"
	"ledger-service/shared/models"
)

// Producer handles publishing transactions to a Kafka topic.
type Producer struct {
	writer *kafka.Writer
}

// NewProducer creates a new Kafka producer.
func NewProducer(brokers []string, topic string) *Producer {
	w := &kafka.Writer{
		Addr:     kafka.TCP(brokers...),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	}
	return &Producer{writer: w}
}

// Publish serializes and sends a transaction to Kafka.
func (p *Producer) Publish(ctx context.Context, txn models.Transaction) error {
	body, err := json.Marshal(txn)
	if err != nil {
		return fmt.Errorf("serialize txn: %w", err)
	}

	err = p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(txn.TxnID),
		Value: body,
	})
	if err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	log.Printf("kafka: published txn %s to topic", txn.TxnID)
	return nil
}

// Close gracefully closes the Kafka writer.
func (p *Producer) Close() error {
	return p.writer.Close()
}
