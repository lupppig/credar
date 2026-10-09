package requestcontext_test

import (
	"context"
	"testing"

	"github.com/lupppig/credar/backend/internal/platform/requestcontext"
)

func TestIdentifiersRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := requestcontext.WithIdentifiers(context.Background(), "request", "correlation")
	if requestcontext.RequestID(ctx) != "request" {
		t.Fatalf("unexpected request ID %q", requestcontext.RequestID(ctx))
	}
	if requestcontext.CorrelationID(ctx) != "correlation" {
		t.Fatalf("unexpected correlation ID %q", requestcontext.CorrelationID(ctx))
	}
}

func TestMissingIdentifiersAreEmpty(t *testing.T) {
	t.Parallel()

	if requestcontext.RequestID(context.Background()) != "" {
		t.Fatal("expected missing request ID to be empty")
	}
	if requestcontext.CorrelationID(context.Background()) != "" {
		t.Fatal("expected missing correlation ID to be empty")
	}
}
