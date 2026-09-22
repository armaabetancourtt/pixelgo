package transfers

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

func (r *PostgresRepository) Create(ctx context.Context, t Transfer) (Transfer, error) {
	userID := auth.UserID(ctx)
	if userID != "" {
		var ownsBoth bool
		err := r.db.QueryRow(
			ctx,
			`SELECT
			  EXISTS(SELECT 1 FROM devices WHERE id = $1 AND user_id = $3::uuid)
			  AND
			  EXISTS(SELECT 1 FROM devices WHERE id = $2 AND user_id = $3::uuid)`,
			t.SourceDeviceID,
			t.DestinationDeviceID,
			userID,
		).Scan(&ownsBoth)
		if err != nil {
			return Transfer{}, err
		}
		if !ownsBoth {
			return Transfer{}, ErrInvalidInput
		}
	}

	_, err := r.db.Exec(
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
			$1, NULLIF($2, '')::uuid, $3, $4, $5::transfer_kind, $6::transfer_status,
			NULLIF($7, ''), NULLIF($8, ''), $9, $10, $11, $12
		)`,
		t.ID,
		userID,
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
	userID := auth.UserID(ctx)

	query := transferSelect + ` FROM transfers`
	args := []any{}
	if userID != "" {
		query += ` WHERE user_id = $1::uuid`
		args = append(args, userID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := r.db.Query(ctx, query, args...)
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
	userID := auth.UserID(ctx)

	query := transferSelect + ` FROM transfers WHERE id = $1`
	args := []any{id}
	if userID != "" {
		query += ` AND user_id = $2::uuid`
		args = append(args, userID)
	}

	t, err := scanTransfer(r.db.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Transfer{}, ErrNotFound
	}
	return t, err
}

func (r *PostgresRepository) Update(ctx context.Context, t Transfer) (Transfer, error) {
	userID := auth.UserID(ctx)

	query := `UPDATE transfers
	 SET status = $2::transfer_status,
	     display_name = NULLIF($3, ''),
	     content_type = NULLIF($4, ''),
	     size_bytes = $5,
	     sha256 = $6,
	     updated_at = $7,
	     completed_at = CASE
	       WHEN $2::transfer_status = 'completed'::transfer_status THEN $7
	       ELSE completed_at
	     END
	 WHERE id = $1`
	args := []any{
		t.ID,
		string(t.Status),
		t.DisplayName,
		t.ContentType,
		t.SizeBytes,
		t.SHA256,
		t.UpdatedAt,
	}
	if userID != "" {
		query += ` AND user_id = $8::uuid`
		args = append(args, userID)
	}

	tag, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return Transfer{}, err
	}
	if tag.RowsAffected() == 0 {
		return Transfer{}, ErrNotFound
	}
	return t, nil
}

const transferSelect = `SELECT
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
	updated_at`

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
