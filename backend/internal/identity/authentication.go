package identity

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrAuthenticationRequired    = errors.New("authentication credentials are required")
	ErrInvalidSession            = errors.New("session is invalid or expired")
	ErrAuthenticationUnavailable = errors.New("authentication service is unavailable")
	ErrIdentityMappingConflict   = errors.New("identity cannot be mapped safely")
	ErrUserDisabled              = errors.New("user is disabled")
)

type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusDisabled Status = "DISABLED"
)

type Credentials struct {
	Cookie       string
	SessionToken string
}

func (credentials Credentials) Empty() bool {
	return strings.TrimSpace(credentials.Cookie) == "" && strings.TrimSpace(credentials.SessionToken) == ""
}

type ExternalIdentity struct {
	ID    string
	Email string
}

type User struct {
	ID               string    `json:"id"`
	KratosIdentityID string    `json:"-"`
	Email            string    `json:"email"`
	Status           Status    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type SessionVerifier interface {
	ValidateSession(context.Context, Credentials) (ExternalIdentity, error)
}

type UserResolver interface {
	Resolve(context.Context, ExternalIdentity) (User, error)
}

type Authenticator struct {
	verifier SessionVerifier
	users    UserResolver
}

func NewAuthenticator(verifier SessionVerifier, users UserResolver) *Authenticator {
	return &Authenticator{verifier: verifier, users: users}
}

func (authenticator *Authenticator) Authenticate(
	ctx context.Context,
	credentials Credentials,
) (User, error) {
	if credentials.Empty() {
		return User{}, ErrAuthenticationRequired
	}

	externalIdentity, err := authenticator.verifier.ValidateSession(ctx, credentials)
	if err != nil {
		return User{}, err
	}

	user, err := authenticator.users.Resolve(ctx, externalIdentity)
	if err != nil {
		return User{}, err
	}
	if user.Status == StatusDisabled {
		return User{}, ErrUserDisabled
	}

	return user, nil
}
