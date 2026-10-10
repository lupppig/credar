// Package requestcontext stores transport identifiers on standard contexts so
// application and infrastructure code can correlate downstream work without a
// dependency on Gin.
package requestcontext

import "context"

type key uint8

const (
	requestIDKey key = iota
	correlationIDKey
	userIDKey
	tenantIDKey
)

// WithIdentifiers returns a child context containing request and correlation
// identifiers.
func WithIdentifiers(parent context.Context, requestID string, correlationID string) context.Context {
	withRequestID := context.WithValue(parent, requestIDKey, requestID)
	return context.WithValue(withRequestID, correlationIDKey, correlationID)
}

// RequestID returns the request identifier, or an empty string when none is
// associated with the context.
func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

// CorrelationID returns the correlation identifier, or an empty string when
// none is associated with the context.
func CorrelationID(ctx context.Context) string {
	value, _ := ctx.Value(correlationIDKey).(string)
	return value
}

// WithUserID returns a child context containing the authenticated application
// user identifier.
func WithUserID(parent context.Context, userID string) context.Context {
	return context.WithValue(parent, userIDKey, userID)
}

// UserID returns the authenticated application user identifier, or an empty
// string when the request is anonymous.
func UserID(ctx context.Context) string {
	value, _ := ctx.Value(userIDKey).(string)
	return value
}

// WithTenantID returns a child context containing the explicitly selected and
// validated tenant identifier.
func WithTenantID(parent context.Context, tenantID string) context.Context {
	return context.WithValue(parent, tenantIDKey, tenantID)
}

// TenantID returns the selected tenant identifier, or an empty string when no
// tenant context was supplied.
func TenantID(ctx context.Context) string {
	value, _ := ctx.Value(tenantIDKey).(string)
	return value
}
