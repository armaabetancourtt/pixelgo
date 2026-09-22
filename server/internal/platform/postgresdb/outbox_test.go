package postgresdb

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestTransferReadyEnqueuesPushOutboxTransactionally(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	suffix := time.Now().UnixNano()
	email := fmt.Sprintf("outbox-%d@pixelgo.local", suffix)
	sourceID := fmt.Sprintf("dev_source_%d", suffix)
	destinationID := fmt.Sprintf("dev_destination_%d", suffix)
	transferID := fmt.Sprintf("tr_outbox_%d", suffix)

	var userID string
	err = tx.QueryRow(
		ctx,
		`INSERT INTO users (email, password_hash)
		 VALUES ($1, 'test-hash')
		 RETURNING id::text`,
		email,
	).Scan(&userID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := tx.Exec(
		ctx,
		`INSERT INTO devices (id, user_id, name, platform, created_at)
		 VALUES
		   ($1, $3::uuid, 'Source', 'ios', now()),
		   ($2, $3::uuid, 'Destination', 'android', now())`,
		sourceID,
		destinationID,
		userID,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE devices SET push_token = 'fcm-test-token' WHERE id = $1`,
		destinationID,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := tx.Exec(
		ctx,
		`INSERT INTO transfers (
			id,
			user_id,
			source_device_id,
			destination_device_id,
			kind,
			status,
			display_name,
			size_bytes,
			sha256,
			created_at,
			updated_at
		) VALUES (
			$1,
			$2::uuid,
			$3,
			$4,
			'file',
			'uploading',
			'example.pdf',
			12,
			'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
			now(),
			now()
		)`,
		transferID,
		userID,
		sourceID,
		destinationID,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE transfers SET status = 'ready', updated_at = now() WHERE id = $1`,
		transferID,
	); err != nil {
		t.Fatal(err)
	}

	var count int
	var eventType string
	var payloadTransferID string
	err = tx.QueryRow(
		ctx,
		`SELECT
			count(*)::int,
			max(event_type),
			max(payload->>'transferId')
		 FROM notification_outbox
		 WHERE transfer_id = $1`,
		transferID,
	).Scan(&count, &eventType, &payloadTransferID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one outbox row, got %d", count)
	}
	if eventType != "transfer.ready" {
		t.Fatalf("expected transfer.ready, got %q", eventType)
	}
	if payloadTransferID != transferID {
		t.Fatalf("expected payload transfer ID %q, got %q", transferID, payloadTransferID)
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE transfers SET status = 'ready', updated_at = now() WHERE id = $1`,
		transferID,
	); err != nil {
		t.Fatal(err)
	}

	if err := tx.QueryRow(
		ctx,
		`SELECT count(*)::int
		 FROM notification_outbox
		 WHERE transfer_id = $1`,
		transferID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected outbox dedupe to keep one row, got %d", count)
	}
}

func TestTransferReadySkipsOutboxWithoutPushToken(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	suffix := time.Now().UnixNano()
	email := fmt.Sprintf("outbox-none-%d@pixelgo.local", suffix)
	sourceID := fmt.Sprintf("dev_source_none_%d", suffix)
	destinationID := fmt.Sprintf("dev_destination_none_%d", suffix)
	transferID := fmt.Sprintf("tr_outbox_none_%d", suffix)

	var userID string
	if err := tx.QueryRow(
		ctx,
		`INSERT INTO users (email, password_hash)
		 VALUES ($1, 'test-hash')
		 RETURNING id::text`,
		email,
	).Scan(&userID); err != nil {
		t.Fatal(err)
	}

	if _, err := tx.Exec(
		ctx,
		`INSERT INTO devices (id, user_id, name, platform, created_at)
		 VALUES
		   ($1, $3::uuid, 'Source', 'ios', now()),
		   ($2, $3::uuid, 'Destination', 'android', now())`,
		sourceID,
		destinationID,
		userID,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := tx.Exec(
		ctx,
		`INSERT INTO transfers (
			id, user_id, source_device_id, destination_device_id,
			kind, status, size_bytes, sha256, created_at, updated_at
		) VALUES (
			$1, $2::uuid, $3, $4,
			'text', 'uploading', 4,
			'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',
			now(), now()
		)`,
		transferID,
		userID,
		sourceID,
		destinationID,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE transfers SET status = 'ready', updated_at = now() WHERE id = $1`,
		transferID,
	); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := tx.QueryRow(
		ctx,
		`SELECT count(*)::int FROM notification_outbox WHERE transfer_id = $1`,
		transferID,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected no outbox row without push token, got %d", count)
	}
}
