package database_test

import (
	"context"
	"testing"

	"github.com/lupppig/credar/backend/internal/platform/database"
)

func TestNewRejectsInvalidPoolPolicy(t *testing.T) {
	t.Parallel()

	tests := []database.Config{
		{ConnectionString: testConnectionString, MaxConnections: 0, MinConnections: 0},
		{ConnectionString: testConnectionString, MaxConnections: 2, MinConnections: -1},
		{ConnectionString: testConnectionString, MaxConnections: 2, MinConnections: 3},
	}
	for _, configuration := range tests {
		if _, err := database.New(configuration); err == nil {
			t.Fatalf("expected invalid pool policy to fail: %#v", configuration)
		}
	}
}

func TestPoolRequiresStartedDatabase(t *testing.T) {
	t.Parallel()

	databaseConnection, err := database.New(database.Config{
		ConnectionString: testConnectionString,
		MaxConnections:   1,
	})
	if err != nil {
		t.Fatalf("construct database: %v", err)
	}

	if _, err := databaseConnection.Pool(); err == nil {
		t.Fatal("expected pool access before startup to fail")
	}
	if err := databaseConnection.Check(context.Background()); err == nil {
		t.Fatal("expected readiness before startup to fail")
	}
}

const testConnectionString = "postgres://user:password@127.0.0.1:5432/database?sslmode=disable"
