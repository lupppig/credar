// Package tenancy owns tenants and the lifecycle of user membership in them.
package tenancy

import (
	"errors"
	"time"
)

var (
	ErrInvalidInput         = errors.New("tenancy input is invalid")
	ErrIdempotencyKeyReused = errors.New("idempotency key was reused for different input")
	ErrTenantNotFound       = errors.New("tenant was not found")
	ErrUserNotFound         = errors.New("application user was not found or active")
	ErrMembershipNotFound   = errors.New("membership was not found")
	ErrMembershipExists     = errors.New("a current membership already exists")
	ErrInvalidTransition    = errors.New("membership status transition is invalid")
	ErrUnavailable          = errors.New("tenancy persistence is unavailable")
)

type TenantStatus string

const (
	TenantStatusActive   TenantStatus = "ACTIVE"
	TenantStatusDisabled TenantStatus = "DISABLED"
)

type MembershipStatus string

const (
	MembershipStatusActive   MembershipStatus = "ACTIVE"
	MembershipStatusDisabled MembershipStatus = "DISABLED"
	MembershipStatusRemoved  MembershipStatus = "REMOVED"
)

type Tenant struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Status    TenantStatus `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type Membership struct {
	ID         string           `json:"id"`
	TenantID   string           `json:"tenant_id"`
	UserID     string           `json:"user_id"`
	Status     MembershipStatus `json:"status"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
	DisabledAt *time.Time       `json:"disabled_at"`
	RemovedAt  *time.Time       `json:"removed_at"`
}

type CreatedTenant struct {
	Tenant            Tenant     `json:"tenant"`
	InitialMembership Membership `json:"initial_membership"`
}

type CreateTenantCommand struct {
	Name           string
	OwnerUserID    string
	IdempotencyKey string
}

type CreateMembershipCommand struct {
	TenantID string
	UserID   string
}

type ChangeMembershipStatusCommand struct {
	TenantID     string
	MembershipID string
	Status       MembershipStatus
	Reason       string
}

func CanTransitionMembership(from MembershipStatus, to MembershipStatus) bool {
	switch from {
	case MembershipStatusActive:
		return to == MembershipStatusDisabled || to == MembershipStatusRemoved
	case MembershipStatusDisabled:
		return to == MembershipStatusActive || to == MembershipStatusRemoved
	default:
		return false
	}
}
