// Package event defines the versioned internal integration-event envelope.
package event

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/lupppig/credar/backend/internal/platform/identifier"
)

var (
	eventTypePattern     = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
	aggregateTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// Aggregate identifies the source aggregate without embedding domain payload.
type Aggregate struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// Envelope is the immutable v1 integration-event transport contract.
type Envelope struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Version       int             `json:"version"`
	OccurredAt    time.Time       `json:"occurred_at"`
	TenantID      *string         `json:"tenant_id"`
	Aggregate     Aggregate       `json:"aggregate"`
	CorrelationID string          `json:"correlation_id"`
	CausationID   string          `json:"causation_id"`
	Data          json.RawMessage `json:"data"`
}

// Validate enforces the shared envelope contract before persistence or
// publication.
func (envelope Envelope) Validate() error {
	identifiers := []struct {
		name  string
		value string
	}{
		{name: "id", value: envelope.ID},
		{name: "aggregate.id", value: envelope.Aggregate.ID},
		{name: "correlation_id", value: envelope.CorrelationID},
		{name: "causation_id", value: envelope.CausationID},
	}
	if envelope.TenantID != nil {
		identifiers = append(identifiers, struct {
			name  string
			value string
		}{name: "tenant_id", value: *envelope.TenantID})
	}
	for _, field := range identifiers {
		if !identifier.IsUUIDv7(field.value) {
			return fmt.Errorf("event %s must be a canonical UUIDv7", field.name)
		}
	}
	if !eventTypePattern.MatchString(envelope.Type) {
		return fmt.Errorf("event type must contain at least two lowercase dot-separated segments")
	}
	if envelope.Version < 1 {
		return fmt.Errorf("event version must be positive")
	}
	if envelope.OccurredAt.IsZero() {
		return fmt.Errorf("event occurred_at must not be zero")
	}
	if !aggregateTypePattern.MatchString(envelope.Aggregate.Type) {
		return fmt.Errorf("event aggregate type must be lowercase snake case")
	}
	if !isJSONObject(envelope.Data) {
		return fmt.Errorf("event data must be a JSON object")
	}
	return nil
}

// Subject returns the stable JetStream subject for this event type and schema
// version.
func (envelope Envelope) Subject() (string, error) {
	if err := envelope.Validate(); err != nil {
		return "", err
	}
	return fmt.Sprintf("credar.events.%s.v%d", envelope.Type, envelope.Version), nil
}

func isJSONObject(data json.RawMessage) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(trimmed, &object) == nil && object != nil
}
