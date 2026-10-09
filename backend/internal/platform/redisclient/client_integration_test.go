//go:build integration

package redisclient_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/lupppig/credar/backend/internal/platform/redisclient"
)

func TestIsolatedEphemeralRoundTrip(t *testing.T) {
	redisURL := os.Getenv("CREDAR_TEST_REDIS_URL")
	keyPrefix := os.Getenv("CREDAR_TEST_REDIS_KEY_PREFIX")
	if redisURL == "" || keyPrefix == "" {
		t.Skip("CREDAR_TEST_REDIS_URL and CREDAR_TEST_REDIS_KEY_PREFIX are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := redisclient.New(redisclient.Config{URL: redisURL, KeyPrefix: keyPrefix})
	if err != nil {
		t.Fatalf("construct Redis client: %v", err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatalf("start Redis client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close(context.Background()) })
	if err := client.Check(ctx); err != nil {
		t.Fatalf("check Redis: %v", err)
	}

	key := "infrastructure-probe"
	want := []byte("ephemeral-value")
	if err := client.Put(ctx, key, want, time.Minute); err != nil {
		t.Fatalf("put ephemeral value: %v", err)
	}
	got, err := client.Get(ctx, key)
	if err != nil {
		t.Fatalf("get ephemeral value: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected ephemeral value %q", got)
	}
	if err := client.Delete(ctx, key); err != nil {
		t.Fatalf("delete ephemeral value: %v", err)
	}
	if _, err := client.Get(ctx, key); !errors.Is(err, redisclient.ErrNotFound) {
		t.Fatalf("expected deleted value to be absent, got %v", err)
	}
}
