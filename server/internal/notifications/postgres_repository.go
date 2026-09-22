package notifications

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Claim(
	ctx context.Context,
	limit int,
) ([]Delivery, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.db.Query(
		ctx,
		`WITH picked AS (
			SELECT id
			FROM notification_outbox
			WHERE sent_at IS NULL
			  AND failed_at IS NULL
			  AND available_at <= now()
			  AND (
			    locked_at IS NULL
			    OR locked_at < now() - interval '5 minutes'
			  )
			ORDER BY id
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		),
		locked AS (
			UPDATE notification_outbox o
			SET locked_at = now()
			FROM picked p
			WHERE o.id = p.id
			RETURNING o.*
		)
		SELECT
			l.id,
			l.device_id,
			l.transfer_id,
			l.event_type,
			d.platform,
			COALESCE(d.push_token, ''),
			l.payload,
			l.attempts
		FROM locked l
		JOIN devices d ON d.id = l.device_id
		ORDER BY l.id`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Delivery, 0)
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(
			&d.ID,
			&d.DeviceID,
			&d.TransferID,
			&d.EventType,
			&d.Platform,
			&d.PushToken,
			&d.Payload,
			&d.Attempts,
		); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepository) MarkSent(
	ctx context.Context,
	id int64,
) error {
	_, err := r.db.Exec(
		ctx,
		`UPDATE notification_outbox
		 SET sent_at = now(),
		     locked_at = NULL,
		     last_error = NULL
		 WHERE id = $1`,
		id,
	)
	return err
}

func (r *PostgresRepository) Retry(
	ctx context.Context,
	id int64,
	message string,
	availableAt time.Time,
	final bool,
) error {
	if final {
		_, err := r.db.Exec(
			ctx,
			`UPDATE notification_outbox
			 SET attempts = attempts + 1,
			     failed_at = now(),
			     locked_at = NULL,
			     last_error = left($2, 2000)
			 WHERE id = $1`,
			id,
			message,
		)
		return err
	}

	_, err := r.db.Exec(
		ctx,
		`UPDATE notification_outbox
		 SET attempts = attempts + 1,
		     available_at = $2,
		     locked_at = NULL,
		     last_error = left($3, 2000)
		 WHERE id = $1`,
		id,
		availableAt,
		message,
	)
	return err
}
