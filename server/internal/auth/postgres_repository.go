package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	var user User
	err := r.db.QueryRow(
		ctx,
		`INSERT INTO users (email, password_hash)
		 VALUES ($1, $2)
		 RETURNING id::text, email, password_hash, created_at`,
		email,
		passwordHash,
	).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return User{}, ErrEmailTaken
		}
		return User{}, err
	}
	return user, nil
}

func (r *PostgresRepository) GetUserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := r.db.QueryRow(
		ctx,
		`SELECT id::text, email, password_hash, created_at
		 FROM users
		 WHERE email = $1`,
		email,
	).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrInvalidCredentials
	}
	return user, err
}

func (r *PostgresRepository) CreateRefresh(
	ctx context.Context,
	userID,
	familyID,
	tokenHash string,
	expiresAt time.Time,
) error {
	_, err := r.db.Exec(
		ctx,
		`INSERT INTO refresh_tokens (
			token_hash,
			user_id,
			family_id,
			expires_at
		) VALUES ($1, $2::uuid, $3, $4)`,
		tokenHash,
		userID,
		familyID,
		expiresAt,
	)
	return err
}

func (r *PostgresRepository) RotateRefresh(
	ctx context.Context,
	tokenHash,
	replacementHash string,
	replacementExpiresAt,
	now time.Time,
) (string, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var userID string
	var familyID string
	var expiresAt time.Time
	var usedAt *time.Time
	var revokedAt *time.Time

	err = tx.QueryRow(
		ctx,
		`SELECT user_id::text, family_id, expires_at, used_at, revoked_at
		 FROM refresh_tokens
		 WHERE token_hash = $1
		 FOR UPDATE`,
		tokenHash,
	).Scan(
		&userID,
		&familyID,
		&expiresAt,
		&usedAt,
		&revokedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidRefresh
	}
	if err != nil {
		return "", err
	}

	if revokedAt != nil || !expiresAt.After(now) {
		return "", ErrInvalidRefresh
	}

	if usedAt != nil {
		if _, err := tx.Exec(
			ctx,
			`UPDATE refresh_tokens
			 SET revoked_at = COALESCE(revoked_at, $2)
			 WHERE family_id = $1`,
			familyID,
			now,
		); err != nil {
			return "", err
		}
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
		return "", ErrRefreshReuse
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE refresh_tokens
		 SET used_at = $2
		 WHERE token_hash = $1`,
		tokenHash,
		now,
	); err != nil {
		return "", err
	}

	if _, err := tx.Exec(
		ctx,
		`INSERT INTO refresh_tokens (
			token_hash,
			user_id,
			family_id,
			expires_at
		) VALUES ($1, $2::uuid, $3, $4)`,
		replacementHash,
		userID,
		familyID,
		replacementExpiresAt,
	); err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return userID, nil
}
