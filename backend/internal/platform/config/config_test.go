package config_test

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lupppig/credar/backend/internal/platform/config"
)

func TestLoadUsesSafeLocalDefaults(t *testing.T) {
	t.Parallel()

	loaded, err := config.Load(emptyEnvironment)
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}

	if loaded.Environment != "local" {
		t.Fatalf("expected local environment, got %q", loaded.Environment)
	}
	if loaded.LogLevel != slog.LevelInfo {
		t.Fatalf("expected info log level, got %s", loaded.LogLevel)
	}
	if loaded.HTTPAddress != ":8080" {
		t.Fatalf("expected :8080 HTTP address, got %q", loaded.HTTPAddress)
	}
	if loaded.ShutdownTimeout != 10*time.Second {
		t.Fatalf("expected 10 second shutdown timeout, got %s", loaded.ShutdownTimeout)
	}
	if loaded.ReadinessTimeout != 2*time.Second {
		t.Fatalf("expected 2 second readiness timeout, got %s", loaded.ReadinessTimeout)
	}
	if loaded.Database.MaxConnections != 10 || loaded.Database.MinConnections != 1 {
		t.Fatalf("unexpected database defaults: %#v", loaded.Database)
	}
	if loaded.Database.URL.Reveal() == "" {
		t.Fatal("expected default database URL")
	}
	if loaded.NATS.URL.Reveal() != "nats://127.0.0.1:4222" {
		t.Fatalf("unexpected default NATS URL")
	}
	if loaded.Outbox.BatchSize != 100 || loaded.Outbox.MaxAttempts != 8 {
		t.Fatalf("unexpected default outbox configuration: %#v", loaded.Outbox)
	}
	if loaded.Redis.KeyPrefix != "credar" || loaded.Redis.URL.Reveal() != "redis://127.0.0.1:6379/0" {
		t.Fatalf("unexpected Redis defaults: %#v", loaded.Redis)
	}
	if loaded.ObjectStorage.Endpoint != "http://127.0.0.1:8333" || loaded.ObjectStorage.Bucket != "credar-local" {
		t.Fatalf("unexpected object-storage defaults: %#v", loaded.ObjectStorage)
	}
}

func TestLoadAcceptsExplicitConfiguration(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"CREDAR_ENVIRONMENT":                   "sandbox",
		"CREDAR_LOG_LEVEL":                     "debug",
		"CREDAR_HTTP_ADDRESS":                  "127.0.0.1:9090",
		"CREDAR_HTTP_SHUTDOWN_TIMEOUT_SECONDS": "15",
		"CREDAR_READINESS_TIMEOUT_SECONDS":     "3",
		"CREDAR_DATABASE_URL":                  "postgres://user:secret@database:5432/testdb?sslmode=disable",
		"CREDAR_DATABASE_MAX_CONNECTIONS":      "20",
		"CREDAR_DATABASE_MIN_CONNECTIONS":      "2",
		"CREDAR_NATS_URL":                      "tls://user:secret@nats.example:4222",
		"CREDAR_OUTBOX_POLL_MILLISECONDS":      "100",
		"CREDAR_OUTBOX_LEASE_SECONDS":          "20",
		"CREDAR_OUTBOX_BATCH_SIZE":             "25",
		"CREDAR_OUTBOX_MAX_ATTEMPTS":           "6",
		"CREDAR_OUTBOX_BASE_BACKOFF_SECONDS":   "2",
		"CREDAR_OUTBOX_MAX_BACKOFF_SECONDS":    "120",
		"CREDAR_REDIS_URL":                     "rediss://user:secret@redis.example:6380/2",
		"CREDAR_REDIS_KEY_PREFIX":              "sandbox_test",
		"CREDAR_OBJECT_STORAGE_ENDPOINT":       "https://objects.example:9443",
		"CREDAR_OBJECT_STORAGE_BUCKET":         "sandbox-objects",
		"CREDAR_OBJECT_STORAGE_ACCESS_KEY":     "object-access",
		"CREDAR_OBJECT_STORAGE_SECRET_KEY":     "object-secret",
	}
	loaded, err := config.Load(mapEnvironment(values))
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}

	if loaded.Environment != "sandbox" || loaded.LogLevel != slog.LevelDebug {
		t.Fatalf("unexpected identity configuration: %#v", loaded)
	}
	if loaded.HTTPAddress != "127.0.0.1:9090" {
		t.Fatalf("unexpected HTTP address %q", loaded.HTTPAddress)
	}
	if loaded.ShutdownTimeout != 15*time.Second || loaded.ReadinessTimeout != 3*time.Second {
		t.Fatalf("unexpected timeouts: %#v", loaded)
	}
	if loaded.Database.MaxConnections != 20 || loaded.Database.MinConnections != 2 {
		t.Fatalf("unexpected database configuration: %#v", loaded.Database)
	}
	if strings.Contains(loaded.Database.URL.String(), "secret") || loaded.Database.URL.String() != "<redacted>" {
		t.Fatalf("database URL was not redacted: %s", loaded.Database.URL)
	}
	if rendered := fmt.Sprintf("%#v", loaded); strings.Contains(rendered, "secret") {
		t.Fatalf("formatted configuration exposed the database password: %s", rendered)
	}
	if loaded.NATS.URL.String() != "<redacted>" {
		t.Fatalf("NATS URL was not redacted: %s", loaded.NATS.URL)
	}
	if loaded.Outbox.PollInterval != 100*time.Millisecond || loaded.Outbox.MaxBackoff != 120*time.Second {
		t.Fatalf("unexpected outbox configuration: %#v", loaded.Outbox)
	}
	if loaded.Redis.URL.String() != "<redacted>" || loaded.Redis.KeyPrefix != "sandbox_test" {
		t.Fatalf("unexpected Redis configuration: %#v", loaded.Redis)
	}
	if loaded.ObjectStorage.AccessKey.String() != "<redacted>" || loaded.ObjectStorage.SecretKey.String() != "<redacted>" {
		t.Fatalf("object-storage credentials were not redacted: %#v", loaded.ObjectStorage)
	}
}

func TestLoadRejectsInvalidValuesWithVariableName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		variable string
		value    string
	}{
		{name: "environment", variable: "CREDAR_ENVIRONMENT", value: "production"},
		{name: "log level", variable: "CREDAR_LOG_LEVEL", value: "verbose"},
		{name: "address", variable: "CREDAR_HTTP_ADDRESS", value: "localhost"},
		{name: "shutdown timeout", variable: "CREDAR_HTTP_SHUTDOWN_TIMEOUT_SECONDS", value: "0"},
		{name: "readiness timeout", variable: "CREDAR_READINESS_TIMEOUT_SECONDS", value: "many"},
		{name: "database URL", variable: "CREDAR_DATABASE_URL", value: "mysql://database/test"},
		{name: "database maximum", variable: "CREDAR_DATABASE_MAX_CONNECTIONS", value: "101"},
		{name: "database minimum", variable: "CREDAR_DATABASE_MIN_CONNECTIONS", value: "11"},
		{name: "NATS URL", variable: "CREDAR_NATS_URL", value: "http://nats.example"},
		{name: "outbox poll", variable: "CREDAR_OUTBOX_POLL_MILLISECONDS", value: "0"},
		{name: "outbox attempts", variable: "CREDAR_OUTBOX_MAX_ATTEMPTS", value: "101"},
		{name: "outbox backoff", variable: "CREDAR_OUTBOX_MAX_BACKOFF_SECONDS", value: "0"},
		{name: "Redis URL", variable: "CREDAR_REDIS_URL", value: "http://redis.example"},
		{name: "Redis prefix", variable: "CREDAR_REDIS_KEY_PREFIX", value: "Bad Prefix"},
		{name: "object endpoint", variable: "CREDAR_OBJECT_STORAGE_ENDPOINT", value: "ftp://objects.example"},
		{name: "object bucket", variable: "CREDAR_OBJECT_STORAGE_BUCKET", value: "UPPERCASE"},
		{name: "object bucket IP", variable: "CREDAR_OBJECT_STORAGE_BUCKET", value: "127.0.0.1"},
		{name: "object access key", variable: "CREDAR_OBJECT_STORAGE_ACCESS_KEY", value: ""},
		{name: "object secret key", variable: "CREDAR_OBJECT_STORAGE_SECRET_KEY", value: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Load(mapEnvironment(map[string]string{test.variable: test.value}))
			if err == nil {
				t.Fatal("expected configuration error")
			}
			if !strings.Contains(err.Error(), test.variable) {
				t.Fatalf("expected error to name %s, got %q", test.variable, err)
			}
		})
	}
}

func emptyEnvironment(string) (string, bool) {
	return "", false
}

func mapEnvironment(values map[string]string) config.LookupEnv {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}
