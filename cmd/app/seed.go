package main

import (
	"context"
	"fmt"
	"time"

	"github.com/qzq-kiim/shop/internal/config"
	"github.com/qzq-kiim/shop/internal/domain/notify"
	"github.com/qzq-kiim/shop/internal/storage/postgres"
)

// demoChatID is the Telegram chat the demo outbox message targets. It only ever
// reaches the fake bot, which writes to the log.
const demoChatID = int64(100000000)

func seed(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg.Env)

	store, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := postgres.NewSeeder(store).Run(ctx, time.Now().UTC()); err != nil {
		return err
	}
	if err := enqueueDemoNotification(ctx, store); err != nil {
		return err
	}
	log.Info("seed complete")
	return nil
}

// enqueueDemoNotification puts one message in the outbox so a freshly seeded
// environment shows the worker doing its job.
func enqueueDemoNotification(ctx context.Context, store *postgres.Store) error {
	payload, err := notify.NewPayload("Demo notification: the outbox worker is running.")
	if err != nil {
		return err
	}
	err = postgres.NewNotifyRepo(store).Enqueue(ctx, notify.Notification{
		ChatID:  demoChatID,
		Kind:    notify.KindOrderLinked,
		Payload: payload,
	}, "seed|demo|outbox")
	if err != nil {
		return fmt.Errorf("enqueue demo notification: %w", err)
	}
	return nil
}
