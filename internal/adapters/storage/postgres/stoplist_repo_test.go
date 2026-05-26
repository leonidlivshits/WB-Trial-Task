package postgres

import (
	"context"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"

	"wbtrialtask/internal/domain"
)

func TestStopListRepository_AddAndList(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Errorf("create sqlmock: %v", err)
		return
	}
	defer db.Close()

	repo := newRepositoryFromDB(db)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO stop_words(word) VALUES ($1)`).
		WithArgs("купить").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE stop_list_state SET version = version + 1, updated_at_ms = $1 WHERE id = 1`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT version FROM stop_list_state WHERE id = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
	mock.ExpectCommit()

	version, err := repo.Add(ctx, "Купить")
	if err != nil {
		t.Errorf("add stop-word: %v", err)
		return
	}
	if version != 1 {
		t.Errorf("unexpected version: got %d want %d", version, 1)
	}

	mock.ExpectQuery(`SELECT word FROM stop_words ORDER BY word ASC`).
		WillReturnRows(sqlmock.NewRows([]string{"word"}).AddRow("купить").AddRow("скидка"))
	mock.ExpectQuery(`SELECT version FROM stop_list_state WHERE id = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(2))

	words, listVersion, err := repo.List(ctx)
	if err != nil {
		t.Errorf("list stop-words: %v", err)
		return
	}
	if len(words) != 2 {
		t.Errorf("unexpected words len: got %d want %d", len(words), 2)
	}
	if listVersion != 2 {
		t.Errorf("unexpected list version: got %d want %d", listVersion, 2)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}

func TestStopListRepository_DomainErrors(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Errorf("create sqlmock: %v", err)
		return
	}
	defer db.Close()

	repo := newRepositoryFromDB(db)
	ctx := context.Background()

	if _, err := repo.Add(ctx, " "); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument on empty add, got %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO stop_words(word) VALUES ($1)`).
		WithArgs("скидка").
		WillReturnError(&pgconn.PgError{Code: "23505"})
	mock.ExpectRollback()
	if _, err := repo.Add(ctx, "скидка"); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM stop_words WHERE word = $1`).
		WithArgs("missing").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	if _, err := repo.Remove(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sql expectations: %v", err)
	}
}
