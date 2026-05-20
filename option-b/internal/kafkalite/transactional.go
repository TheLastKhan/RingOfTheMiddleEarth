package kafkalite

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// TransactionalProducer writes records with Kafka producer transactions.
type TransactionalProducer struct {
	mu     sync.Mutex
	client *kgo.Client
}

func NewTransactionalProducer(brokers, transactionalID string) (*TransactionalProducer, error) {
	if transactionalID == "" {
		return nil, errors.New("transactional id is required")
	}
	brokerList := NewProducer(brokers).brokers
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokerList...),
		kgo.TransactionalID(transactionalID),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordPartitioner(kgo.ManualPartitioner()),
		kgo.MaxProduceRequestsInflightPerBroker(1),
		kgo.TransactionTimeout(30*time.Second),
	)
	if err != nil {
		return nil, err
	}
	return &TransactionalProducer{client: client}, nil
}

func (p *TransactionalProducer) Close() {
	if p != nil && p.client != nil {
		p.client.Close()
	}
}

// ProduceTransaction commits one record atomically. If any step fails, the
// buffered transaction is aborted before returning.
func (p *TransactionalProducer) ProduceTransaction(ctx context.Context, topic, key string, value []byte) error {
	if p == nil || p.client == nil {
		return errors.New("transactional producer is not configured")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.client.BeginTransaction(); err != nil {
		return err
	}

	record := &kgo.Record{
		Topic:     topic,
		Key:       []byte(key),
		Value:     value,
		Partition: partitionFor(topic),
	}
	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		_ = p.client.AbortBufferedRecords(context.Background())
		_ = p.client.EndTransaction(context.Background(), kgo.TryAbort)
		return err
	}
	if err := p.client.Flush(ctx); err != nil {
		_ = p.client.AbortBufferedRecords(context.Background())
		_ = p.client.EndTransaction(context.Background(), kgo.TryAbort)
		return err
	}
	if err := p.client.EndTransaction(context.Background(), kgo.TryCommit); err != nil {
		_ = p.client.EndTransaction(context.Background(), kgo.TryAbort)
		return err
	}
	return nil
}
