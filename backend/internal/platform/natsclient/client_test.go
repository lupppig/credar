package natsclient_test

import (
	"context"
	"testing"

	"github.com/lupppig/credar/backend/internal/platform/natsclient"
)

func TestClientRequiresStartupBeforeUse(t *testing.T) {
	t.Parallel()

	client, err := natsclient.New("nats://127.0.0.1:4222")
	if err != nil {
		t.Fatalf("construct NATS client: %v", err)
	}
	if err := client.Check(context.Background()); err == nil {
		t.Fatal("expected readiness before startup to fail")
	}
	if _, err := client.JetStream(); err == nil {
		t.Fatal("expected JetStream access before startup to fail")
	}
}
