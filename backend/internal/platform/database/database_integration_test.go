//go:build integration

package database_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lupppig/credar/backend/internal/platform/database"
)

func TestPostgreSQLLifecycleAndTransactions(t *testing.T) {
	connectionString := os.Getenv("CREDAR_TEST_DATABASE_URL")
	if connectionString == "" {
		t.Skip("CREDAR_TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	databaseConnection, err := database.New(database.Config{
		ConnectionString: connectionString,
		MaxConnections:   1,
		MinConnections:   0,
	})
	if err != nil {
		t.Fatalf("construct database: %v", err)
	}
	if err := databaseConnection.Start(ctx); err != nil {
		t.Fatalf("start database: %v", err)
	}
	t.Cleanup(func() {
		if err := databaseConnection.Close(context.Background()); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	if err := databaseConnection.Check(ctx); err != nil {
		t.Fatalf("check database readiness: %v", err)
	}

	pool, err := databaseConnection.Pool()
	if err != nil {
		t.Fatalf("get pool: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE TEMP TABLE transaction_probe (value integer NOT NULL)`); err != nil {
		t.Fatalf("create transaction probe: %v", err)
	}

	if err := databaseConnection.InTransaction(ctx, pgx.TxOptions{}, func(transaction database.DBTX) error {
		_, err := transaction.Exec(ctx, `INSERT INTO transaction_probe (value) VALUES (1)`)
		return err
	}); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}

	rollback := errors.New("request rollback")
	err = databaseConnection.InTransaction(ctx, pgx.TxOptions{}, func(transaction database.DBTX) error {
		if _, err := transaction.Exec(ctx, `INSERT INTO transaction_probe (value) VALUES (2)`); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("expected rollback cause, got %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM transaction_probe`).Scan(&count); err != nil {
		t.Fatalf("count committed rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one committed row, got %d", count)
	}
}
