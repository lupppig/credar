// Package authorization owns roles, permissions, assignments, and policy evaluation.
package authorization

import (
	"errors"
	"time"

	"github.com/lupppig/credar/backend/internal/identity"
	"github.com/lupppig/credar/backend/internal/tenancy"
)

const (
	PlatformAdminRoleID = "01900000-0000-7000-8000-000000000001"
	TenantOwnerRoleID   = "01900000-0000-7000-8000-000000000002"
	TenantAdminRoleID   = "01900000-0000-7000-8000-000000000003"
	TenantAuditorRoleID = "01900000-0000-7000-8000-000000000004"
	TenantMemberRoleID  = "01900000-0000-7000-8000-000000000005"
)

var (
	ErrPermissionDenied          = errors.New("permission denied")
	ErrResourceNotFound          = errors.New("resource not found")
	ErrInvalidInput              = errors.New("authorization input is invalid")
	ErrSystemRoleImmutable       = errors.New("system role is immutable")
	ErrPermissionScopeEscalation = errors.New("permission scope escalation")
	ErrLastTenantOwnerRequired   = errors.New("last active tenant owner is required")
	ErrLastPlatformAdminRequired = errors.New("last active platform administrator is required")
	ErrRoleAssigned              = errors.New("role is assigned")
	ErrBootstrapAlreadyComplete  = errors.New("platform administrator bootstrap is already complete")
	ErrUnavailable               = errors.New("authorization persistence is unavailable")
)

type Scope string
type Kind string

const (
	ScopePlatform Scope = "PLATFORM"
	ScopeTenant   Scope = "TENANT"
	KindSystem    Kind  = "SYSTEM"
	KindCustom    Kind  = "CUSTOM"
)

type Permission struct {
	Name        string `json:"name"`
	Scope       Scope  `json:"scope"`
	Description string `json:"description"`
}

type Role struct {
	ID          string       `json:"id"`
	TenantID    *string      `json:"tenant_id"`
	Name        string       `json:"name"`
	Scope       Scope        `json:"scope"`
	Kind        Kind         `json:"kind"`
	Permissions []Permission `json:"permissions"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type Membership struct {
	ID         string                   `json:"id"`
	TenantID   string                   `json:"tenant_id"`
	User       identity.User            `json:"user"`
	Status     tenancy.MembershipStatus `json:"status"`
	Roles      []Role                   `json:"roles"`
	CreatedAt  time.Time                `json:"created_at"`
	UpdatedAt  time.Time                `json:"updated_at"`
	DisabledAt *time.Time               `json:"disabled_at"`
	RemovedAt  *time.Time               `json:"removed_at"`
}

type AuditActor struct {
	Type string  `json:"type"`
	ID   *string `json:"id"`
}

type AuditEvent struct {
	ID            string         `json:"id"`
	Action        string         `json:"action"`
	Actor         AuditActor     `json:"actor"`
	TenantID      *string        `json:"tenant_id"`
	TargetType    string         `json:"target_type"`
	TargetID      string         `json:"target_id"`
	OccurredAt    time.Time      `json:"occurred_at"`
	CorrelationID string         `json:"correlation_id"`
	Reason        *string        `json:"reason,omitempty"`
	Changes       map[string]any `json:"changes,omitempty"`
}

type Page struct {
	NextCursor *string `json:"next_cursor,omitempty"`
	HasMore    bool    `json:"has_more"`
}

type RoleWrite struct {
	Name            string
	PermissionNames []string
	Reason          string
}
