package redisclient_test

import (
	"context"
	"testing"
	"time"

	"github.com/lupppig/credar/backend/internal/platform/redisclient"
)

func TestClientRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		configuration redisclient.Config
	}{
		{name: "URL", configuration: redisclient.Config{URL: "://bad", KeyPrefix: "test"}},
		{name: "prefix", configuration: redisclient.Config{URL: "redis://127.0.0.1:6379/0", KeyPrefix: "bad:prefix"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := redisclient.New(test.configuration); err == nil {
				t.Fatal("expected invalid Redis configuration to fail")
			}
		})
	}
}

func TestClientRequiresStartupAndExpiringValues(t *testing.T) {
	t.Parallel()

	client, err := redisclient.New(redisclient.Config{
		URL:       "redis://127.0.0.1:6379/0",
		KeyPrefix: "test",
	})
	if err != nil {
		t.Fatalf("construct Redis client: %v", err)
	}
	if err := client.Put(context.Background(), "probe", []byte("value"), 0); err == nil {
		t.Fatal("expected non-expiring value to fail")
	}
	if err := client.Put(context.Background(), "probe", []byte("value"), time.Minute); err == nil {
		t.Fatal("expected unstarted client to fail")
	}
}
