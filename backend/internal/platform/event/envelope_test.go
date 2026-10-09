package event_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/lupppig/credar/backend/internal/platform/event"
)

const (
	eventID       = "01991a4b-fa00-7000-8000-000000000001"
	aggregateID   = "01991a4b-fa00-7000-8000-000000000002"
	correlationID = "01991a4b-fa00-7000-8000-000000000003"
	causationID   = "01991a4b-fa00-7000-8000-000000000004"
)

func TestEnvelopeValidationAndSubject(t *testing.T) {
	t.Parallel()

	envelope := validEnvelope()
	if err := envelope.Validate(); err != nil {
		t.Fatalf("validate envelope: %v", err)
	}
	subject, err := envelope.Subject()
	if err != nil {
		t.Fatalf("create subject: %v", err)
	}
	if subject != "credar.events.infrastructure.probe.v1" {
		t.Fatalf("unexpected subject %q", subject)
	}
}

func TestEnvelopeRejectsInvalidFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*event.Envelope)
	}{
		{name: "identifier", mutate: func(value *event.Envelope) { value.ID = "not-a-uuid" }},
		{name: "type", mutate: func(value *event.Envelope) { value.Type = "probe" }},
		{name: "version", mutate: func(value *event.Envelope) { value.Version = 0 }},
		{name: "time", mutate: func(value *event.Envelope) { value.OccurredAt = time.Time{} }},
		{name: "aggregate type", mutate: func(value *event.Envelope) { value.Aggregate.Type = "InfrastructureProbe" }},
		{name: "data", mutate: func(value *event.Envelope) { value.Data = json.RawMessage(`[]`) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			envelope := validEnvelope()
			test.mutate(&envelope)
			if err := envelope.Validate(); err == nil {
				t.Fatal("expected invalid envelope to fail")
			}
		})
	}
}

func validEnvelope() event.Envelope {
	return event.Envelope{
		ID:         eventID,
		Type:       "infrastructure.probe",
		Version:    1,
		OccurredAt: time.Date(2026, 9, 24, 12, 0, 0, 123, time.UTC),
		Aggregate: event.Aggregate{
			Type: "infrastructure_probe",
			ID:   aggregateID,
		},
		CorrelationID: correlationID,
		CausationID:   causationID,
		Data:          json.RawMessage(`{"probe":"committed"}`),
	}
}
