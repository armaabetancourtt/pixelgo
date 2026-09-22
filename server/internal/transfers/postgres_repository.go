package transfers

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, t Transfer) (Transfer, error) {
	_, err := r.db.Exec(
		ctx,
		`INSERT INTO transfers (
			id,
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
			$1, $2, $3, $4, $5, NULLIF($6, ''), NULLIF($7, ''), $8, $9, $10, $11
		)`,
		t.ID,
		t.SourceDeviceID,
		t.DestinationDeviceID,
		string(t.Kind),
		string(t.Status),
		t.DisplayName,
		t.ContentType,
		t.SizeBytes,
		t.SHA256,
		t.CreatedAt,
		t.UpdatedAt,
	)
	if err != nil {
		return Transfer{}, err
	}
	return t, nil
}

func (r *PostgresRepository) List(ctx context.Context) ([]Transfer, error) {
	rows, err := r.db.Query(
		ctx,
		`SELECT
			id,
			source_device_id,
			destination_device_id,
			kind::text,
			status::text,
			COALESCE(display_name, ''),
			COALESCE(content_type, ''),
			size_bytes,
			sha256,
			created_at,
			updated_at
		 FROM transfers
		 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Transfer
	for rows.Next() {
		t, err := scanTransfer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepository) Get(ctx context.Context, id string) (Transfer, error) {
	row := r.db.QueryRow(
		ctx,
		`SELECT
			id,
			source_device_id,
			destination_device_id,
			kind::text,
			status::text,
			COALESCE(display_name, ''),
			COALESCE(content_type, ''),
			size_bytes,
			sha256,
			created_at,
			updated_at
		 FROM transfers
		 WHERE id = $1`,
		id,
	)

	t, err := scanTransfer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Transfer{}, ErrNotFound
	}
	return t, err
}

func (r *PostgresRepository) Update(ctx context.Context, t Transfer) (Transfer, error) {
	tag, err := r.db.Exec(
		ctx,
		`UPDATE transfers
		 SET status = $2,
		     display_name = NULLIF($3, ''),
		     content_type = NULLIF($4, ''),
		     size_bytes = $5,
		     sha256 = $6,
		     updated_at = $7,
		     completed_at = CASE WHEN $2 = 'completed' THEN $7 ELSE completed_at END
		 WHERE id = $1`,
		t.ID,
		string(t.Status),
		t.DisplayName,
		t.ContentType,
		t.SizeBytes,
		t.SHA256,
		t.UpdatedAt,
	)
	if err != nil {
		return Transfer{}, err
	}
	if tag.RowsAffected() == 0 {
		return Transfer{}, ErrNotFound
	}
	return t, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTransfer(row rowScanner) (Transfer, error) {
	var t Transfer
	var kind string
	var status string

	if err := row.Scan(
		&t.ID,
		&t.SourceDeviceID,
		&t.DestinationDeviceID,
		&kind,
		&status,
		&t.DisplayName,
		&t.ContentType,
		&t.SizeBytes,
		&t.SHA256,
		&t.CreatedAt,
		&t.UpdatedAt,
	); err != nil {
		return Transfer{}, err
	}

	t.Kind = Kind(kind)
	t.Status = Status(status)
	return t, nil
}
