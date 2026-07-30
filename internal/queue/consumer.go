package queue

import (
	"context"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const defaultPollInterval = 2 * time.Second

type QueueMessage struct {
	ID              string         `gorm:"primaryKey;size:255"`
	ResourceID      string         `gorm:"size:255;not null"`
	Kind            string         `gorm:"size:100;not null"`
	TargetAdapter   string         `gorm:"size:100;not null"`
	Href            string         `gorm:"size:500"`
	Generation      int64          `gorm:"not null"`
	OwnerReferences datatypes.JSON `gorm:"type:jsonb"`
	EventType       string         `gorm:"size:255;not null"`
	CreatedAt       time.Time      `gorm:"not null"`
}

func (QueueMessage) TableName() string {
	return "reconciliation_queue"
}

type Consumer interface {
	Start(ctx context.Context, kind, adapterName string, handler func(ctx context.Context, msg *QueueMessage) error) error
}

type consumer struct {
	db           *gorm.DB
	log          logger.Logger
	pollInterval time.Duration
}

type ConsumerOption func(*consumer)

func WithPollInterval(d time.Duration) ConsumerOption {
	return func(c *consumer) {
		c.pollInterval = d
	}
}

func NewConsumer(db *gorm.DB, log logger.Logger, opts ...ConsumerOption) Consumer {
	c := &consumer{
		db:           db,
		log:          log,
		pollInterval: defaultPollInterval,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *consumer) Start(ctx context.Context, kind, adapterName string, handler func(ctx context.Context, msg *QueueMessage) error) error {
	c.log.Infof(ctx, "Queue consumer started: kind=%s adapter=%s poll_interval=%s", kind, adapterName, c.pollInterval)
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			c.log.Info(ctx, "Queue consumer shutting down")
			return ctx.Err()
		case <-ticker.C:
			c.processNext(ctx, kind, adapterName, handler)
		}
	}
}

func (c *consumer) processNext(ctx context.Context, kind, adapterName string, handler func(ctx context.Context, msg *QueueMessage) error) {
	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var msg QueueMessage
		result := tx.Raw(
			"SELECT * FROM reconciliation_queue WHERE kind = ? AND target_adapter = ? ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED",
			kind, adapterName,
		).Scan(&msg)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}

		_ = handler(ctx, &msg)

		return tx.Delete(&QueueMessage{}, "id = ?", msg.ID).Error
	})
	if err != nil {
		errCtx := logger.WithErrorField(ctx, err)
		c.log.Warn(errCtx, "Queue consumer transaction error")
	}
}
