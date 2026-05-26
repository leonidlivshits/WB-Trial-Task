package postgres

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"wbtrialtask/internal/domain"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type StopListRepository struct {
	db *sql.DB
}

func NewStopListRepository(ctx context.Context, dsn string) (*StopListRepository, error) {
	connString := strings.TrimSpace(dsn)
	if connString == "" {
		return nil, fmt.Errorf("postgres stop-list dsn is empty")
	}

	db, err := sql.Open("pgx", connString)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	repo := newRepositoryFromDB(db)
	if err := repo.ping(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := repo.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return repo, nil
}

func (r *StopListRepository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *StopListRepository) Add(ctx context.Context, word string) (int64, error) {
	word = normalizeWord(word)
	if word == "" {
		return 0, domain.ErrInvalidArgument
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `INSERT INTO stop_words(word) VALUES ($1)`, word); err != nil {
		if isUniqueViolation(err) {
			return 0, domain.ErrAlreadyExists
		}
		return 0, fmt.Errorf("insert stop-word: %w", err)
	}

	version, err := bumpVersion(ctx, tx)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit add stop-word: %w", err)
	}
	return version, nil
}

func (r *StopListRepository) Remove(ctx context.Context, word string) (int64, error) {
	word = normalizeWord(word)
	if word == "" {
		return 0, domain.ErrInvalidArgument
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `DELETE FROM stop_words WHERE word = $1`, word)
	if err != nil {
		return 0, fmt.Errorf("delete stop-word: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return 0, domain.ErrNotFound
	}

	version, err := bumpVersion(ctx, tx)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit remove stop-word: %w", err)
	}
	return version, nil
}

func (r *StopListRepository) List(ctx context.Context) ([]string, int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT word FROM stop_words ORDER BY word ASC`)
	if err != nil {
		return nil, 0, fmt.Errorf("select stop-words: %w", err)
	}
	defer rows.Close()

	words := make([]string, 0)
	for rows.Next() {
		var word string
		if err := rows.Scan(&word); err != nil {
			return nil, 0, fmt.Errorf("scan stop-word: %w", err)
		}
		words = append(words, word)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate stop-words: %w", err)
	}

	var version int64
	if err := r.db.QueryRowContext(ctx, `SELECT version FROM stop_list_state WHERE id = 1`).Scan(&version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return words, 0, nil
		}
		return nil, 0, fmt.Errorf("read stop-list version: %w", err)
	}

	return words, version, nil
}

func (r *StopListRepository) ping(ctx context.Context) error {
	if err := r.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	return nil
}

func (r *StopListRepository) migrate(ctx context.Context) error {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		raw, err := fs.ReadFile(migrationsFS, "migrations/"+name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := r.db.ExecContext(ctx, string(raw)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}

	return nil
}

func bumpVersion(ctx context.Context, tx *sql.Tx) (int64, error) {
	nowMS := time.Now().UTC().UnixMilli()
	if _, err := tx.ExecContext(ctx, `UPDATE stop_list_state SET version = version + 1, updated_at_ms = $1 WHERE id = 1`, nowMS); err != nil {
		return 0, fmt.Errorf("update stop-list version: %w", err)
	}

	var version int64
	if err := tx.QueryRowContext(ctx, `SELECT version FROM stop_list_state WHERE id = 1`).Scan(&version); err != nil {
		return 0, fmt.Errorf("read updated stop-list version: %w", err)
	}
	return version, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func normalizeWord(word string) string {
	return strings.TrimSpace(strings.ToLower(word))
}

func newRepositoryFromDB(db *sql.DB) *StopListRepository {
	return &StopListRepository{db: db}
}
