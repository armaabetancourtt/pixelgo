package devices

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, device Device) (Device, error) {
	_, err := r.db.Exec(
		ctx,
		`INSERT INTO devices (id, name, platform, push_token, created_at)
		 VALUES ($1, $2, $3, NULLIF($4, ''), $5)`,
		device.ID,
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
	rows, err := r.db.Query(
		ctx,
		`SELECT id, name, platform, COALESCE(push_token, ''), created_at
		 FROM devices
		 ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Device
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

func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM devices WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
