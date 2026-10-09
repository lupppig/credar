// Package logging constructs the process-wide structured logger.
package logging

import (
	"io"
	"log/slog"
)

// New returns a JSON logger with stable service and environment attributes.
func New(output io.Writer, level slog.Level, environment string) *slog.Logger {
	handler := slog.NewJSONHandler(output, &slog.HandlerOptions{Level: level})
	return slog.New(handler).With(
		"service", "credar-api",
		"environment", environment,
	)
}
