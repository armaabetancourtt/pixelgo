package devices

import (
	"context"
	"errors"

	"github.com/armaabetancourtt/pixelgo/server/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, device Device) (Device, error) {
	userID := auth.UserID(ctx)
	_, err := r.db.Exec(
		ctx,
		`INSERT INTO devices (id, user_id, name, platform, push_token, created_at)
		 VALUES ($1, NULLIF($2, '')::uuid, $3, $4, NULLIF($5, ''), $6)`,
		device.ID,
		userID,
		device.Name,
		device.Platform,
		device.PushToken,
		device.CreatedAt,
	)
	if err != nil {
		return Device{}, err
	}
	return device, nil
}

func (r *PostgresRepository) List(ctx context.Context) ([]Device, error) {
	userID := auth.UserID(ctx)

	query := `SELECT id, name, platform, COALESCE(push_token, ''), created_at
	          FROM devices`
	args := []any{}
	if userID != "" {
		query += ` WHERE user_id = $1::uuid`
		args = append(args, userID)
	}
	query += ` ORDER BY created_at ASC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Device, 0)
	for rows.Next() {
		var device Device
		if err := rows.Scan(
			&device.ID,
			&device.Name,
			&device.Platform,
			&device.PushToken,
			&device.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, device)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepository) Get(ctx context.Context, id string) (Device, error) {
	userID := auth.UserID(ctx)

	query := `SELECT id, name, platform, COALESCE(push_token, ''), created_at
	          FROM devices
	          WHERE id = $1`
	args := []any{id}
	if userID != "" {
		query += ` AND user_id = $2::uuid`
		args = append(args, userID)
	}

	var device Device
	err := r.db.QueryRow(ctx, query, args...).Scan(
		&device.ID,
		&device.Name,
		&device.Platform,
		&device.PushToken,
		&device.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	return device, err
}

func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	userID := auth.UserID(ctx)

	query := `DELETE FROM devices WHERE id = $1`
	args := []any{id}
	if userID != "" {
		query += ` AND user_id = $2::uuid`
		args = append(args, userID)
	}

	tag, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
