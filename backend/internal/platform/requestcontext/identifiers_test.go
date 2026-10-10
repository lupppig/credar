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
	if requestcontext.UserID(context.Background()) != "" {
		t.Fatal("expected missing user ID to be empty")
	}
	if requestcontext.TenantID(context.Background()) != "" {
		t.Fatal("expected missing tenant ID to be empty")
	}
}

func TestTenantIDRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := requestcontext.WithTenantID(context.Background(), "tenant")
	if requestcontext.TenantID(ctx) != "tenant" {
		t.Fatalf("unexpected tenant ID %q", requestcontext.TenantID(ctx))
	}
}

func TestUserIDRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := requestcontext.WithUserID(context.Background(), "user")
	if requestcontext.UserID(ctx) != "user" {
		t.Fatalf("unexpected user ID %q", requestcontext.UserID(ctx))
	}
}
