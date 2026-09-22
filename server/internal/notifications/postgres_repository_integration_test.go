package notifications

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/armaabetancourtt/pixelgo/server/internal/platform/postgresdb"
)

func TestPostgresOutboxTriggerClaimAndMarkSent(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := postgresdb.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := postgresdb.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	email := "push-outbox-" + suffix + "@pixelgo.local"
	deviceID := "dev_push_" + suffix
	transferID := "tr_push_" + suffix
	checksum := strings.Repeat("a", 64)

	var userID string
	if err := pool.QueryRow(
		ctx,
		`INSERT INTO users (email, password_hash)
		 VALUES ($1, 'integration-test')
		 RETURNING id::text`,
		email,
	).Scan(&userID); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cleanupCancel()

		_, _ = pool.Exec(
			cleanupCtx,
			`DELETE FROM notification_outbox WHERE transfer_id = $1`,
			transferID,
		)
		_, _ = pool.Exec(
			cleanupCtx,
			`DELETE FROM transfers WHERE id = $1`,
			transferID,
		)
		_, _ = pool.Exec(
			cleanupCtx,
			`DELETE FROM devices WHERE id = $1`,
			deviceID,
		)
		_, _ = pool.Exec(
			cleanupCtx,
			`DELETE FROM users WHERE id = $1::uuid`,
			userID,
		)
	})

	if _, err := pool.Exec(
		ctx,
		`INSERT INTO devices (
			id,
			user_id,
			name,
			platform,
			push_token,
			created_at
		) VALUES ($1, $2::uuid, 'Integration iPhone', 'ios', 'push-token', now())`,
		deviceID,
		userID,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(
		ctx,
		`INSERT INTO transfers (
			id,
			user_id,
			source_device_id,
			destination_device_id,
			kind,
			status,
			display_name,
			content_type,
			size_bytes,
			sha256,
			created_at,
			updated_at
		) VALUES (
			$1,
			$2::uuid,
			$3,
			$3,
			'text',
			'uploading',
			'Integration message',
			'text/plain',
			4,
			$4,
			now(),
			now()
		)`,
		transferID,
		userID,
		deviceID,
		checksum,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(
		ctx,
		`UPDATE transfers
		 SET status = 'ready',
		     updated_at = now()
		 WHERE id = $1`,
		transferID,
	); err != nil {
		t.Fatal(err)
	}

	var queued int
	if err := pool.QueryRow(
		ctx,
		`SELECT count(*)
		 FROM notification_outbox
		 WHERE transfer_id = $1
		   AND device_id = $2
		   AND event_type = 'transfer.ready'`,
		transferID,
		deviceID,
	).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("expected one transactional outbox row, got %d", queued)
	}

	repo := NewPostgresRepository(pool)

	first, err := repo.Claim(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 {
		t.Fatalf("expected one claimed delivery, got %d", len(first))
	}

	delivery := first[0]
	if delivery.TransferID != transferID {
		t.Fatalf(
			"expected transfer %q, got %q",
			transferID,
			delivery.TransferID,
		)
	}
	if delivery.DeviceID != deviceID {
		t.Fatalf(
			"expected device %q, got %q",
			deviceID,
			delivery.DeviceID,
		)
	}
	if delivery.Platform != "ios" || delivery.PushToken != "push-token" {
		t.Fatalf(
			"unexpected route: platform=%q token=%q",
			delivery.Platform,
			delivery.PushToken,
		)
	}

	second, err := repo.Claim(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf(
			"expected locked delivery to be excluded by SKIP LOCKED semantics, got %d",
			len(second),
		)
	}

	if err := repo.MarkSent(ctx, delivery.ID); err != nil {
		t.Fatal(err)
	}

	var sent bool
	if err := pool.QueryRow(
		ctx,
		`SELECT sent_at IS NOT NULL
		 FROM notification_outbox
		 WHERE id = $1`,
		delivery.ID,
	).Scan(&sent); err != nil {
		t.Fatal(err)
	}
	if !sent {
		t.Fatal("expected outbox row to be marked sent")
	}

	third, err := repo.Claim(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(third) != 0 {
		t.Fatalf("sent delivery must never be reclaimed, got %d", len(third))
	}
}
