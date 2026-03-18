// Package consumer provides consumers to ingest transactions and route them.
package consumer

import (
	"context"
	"encoding/json"
	"log"

	"github.com/segmentio/kafka-go"
	"ledger-service/coordinator/router"
	"ledger-service/shared/constants"
	"ledger-service/shared/models"
)

// KafkaConsumer reads transactions from Kafka and routes them.
type KafkaConsumer struct {
	reader *kafka.Reader
	router *router.Router
}

// NewKafkaConsumer initializes a Kafka consumer.
func NewKafkaConsumer(brokers []string, topic, groupID string, r *router.Router) *KafkaConsumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
	})
	return &KafkaConsumer{
		reader: reader,
		router: r,
	}
}

// Start begins consuming messages in a loop.
func (c *KafkaConsumer) Start(ctx context.Context) {
	log.Printf("kafka consumer: starting to read from topic")
	go func() {
		for {
			m, err := c.reader.ReadMessage(ctx)
			if err != nil {
				// Context cancelled or closed
				log.Printf("kafka consumer stopped: %v", err)
				return
			}
			
			var txn models.Transaction
			if err := json.Unmarshal(m.Value, &txn); err != nil {
				log.Printf("kafka consumer: failed to unmarshal txn: %v", err)
				continue
			}

			// Validate and route
			txn.State = constants.StatePending
			_, err = c.router.Route(txn)
			if err != nil {
				log.Printf("kafka consumer: txn %s route failed: %v", txn.TxnID, err)
			} else {
				log.Printf("kafka consumer: txn %s successfully routed", txn.TxnID)
			}
		}
	}()
}

// Stop gracefully closes the Kafka reader.
func (c *KafkaConsumer) Stop() error {
	return c.reader.Close()
}
