package identity

import (
	"context"
	"errors"
)

var (
	ErrInvalidTenantContext     = errors.New("tenant context is invalid")
	ErrMembershipInactive       = errors.New("tenant membership is inactive or absent")
	ErrTenantContextUnavailable = errors.New("tenant context is unavailable")
)

type TenantSummary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type MembershipSummary struct {
	ID        string        `json:"id"`
	Tenant    TenantSummary `json:"tenant"`
	Status    string        `json:"status"`
	RoleNames []string      `json:"role_names"`
}

type SessionContextResolver interface {
	ResolveSessionContext(
		ctx context.Context,
		userID string,
		selectedTenantID string,
	) ([]MembershipSummary, *string, error)
}
