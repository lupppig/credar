package outbox_test

import (
	"context"
	"testing"

	"github.com/lupppig/credar/backend/internal/platform/event"
	"github.com/lupppig/credar/backend/internal/platform/outbox"
)

func TestInsertRejectsInvalidEnvelopeBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()

	repository := outbox.NewRepository(nil)
	err := repository.Insert(context.Background(), nil, event.Envelope{})
	if err == nil {
		t.Fatal("expected invalid envelope to fail")
	}
}
