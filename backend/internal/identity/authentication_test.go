package identity

import (
	"context"
	"errors"
	"testing"
)

type verifierStub struct {
	identity ExternalIdentity
	err      error
	calls    int
}

func (stub *verifierStub) ValidateSession(context.Context, Credentials) (ExternalIdentity, error) {
	stub.calls++
	return stub.identity, stub.err
}

type resolverStub struct {
	user  User
	err   error
	calls int
}

func (stub *resolverStub) Resolve(context.Context, ExternalIdentity) (User, error) {
	stub.calls++
	return stub.user, stub.err
}

func TestAuthenticatorRequiresCredentialsBeforeCallingKratos(t *testing.T) {
	verifier := &verifierStub{}
	resolver := &resolverStub{}
	authenticator := NewAuthenticator(verifier, resolver)

	_, err := authenticator.Authenticate(context.Background(), Credentials{})
	if !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatalf("expected authentication required, got %v", err)
	}
	if verifier.calls != 0 || resolver.calls != 0 {
		t.Fatalf("expected no dependency calls, got verifier=%d resolver=%d", verifier.calls, resolver.calls)
	}
}

func TestAuthenticatorReturnsActiveMappedUser(t *testing.T) {
	verifier := &verifierStub{identity: ExternalIdentity{ID: "kratos-id", Email: "person@example.com"}}
	resolver := &resolverStub{user: User{ID: "user-id", Status: StatusActive}}
	authenticator := NewAuthenticator(verifier, resolver)

	user, err := authenticator.Authenticate(context.Background(), Credentials{SessionToken: "opaque-token"})
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if user.ID != "user-id" {
		t.Fatalf("expected mapped user, got %+v", user)
	}
}

func TestAuthenticatorRejectsDisabledUser(t *testing.T) {
	verifier := &verifierStub{identity: ExternalIdentity{ID: "kratos-id", Email: "person@example.com"}}
	resolver := &resolverStub{user: User{ID: "user-id", Status: StatusDisabled}}
	authenticator := NewAuthenticator(verifier, resolver)

	_, err := authenticator.Authenticate(context.Background(), Credentials{Cookie: "ory_kratos_session=value"})
	if !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("expected disabled user, got %v", err)
	}
}

func TestAuthenticatorPreservesBoundaryErrors(t *testing.T) {
	tests := []struct {
		name        string
		verifierErr error
		resolverErr error
		expected    error
	}{
		{name: "invalid session", verifierErr: ErrInvalidSession, expected: ErrInvalidSession},
		{name: "authentication unavailable", verifierErr: ErrAuthenticationUnavailable, expected: ErrAuthenticationUnavailable},
		{name: "mapping conflict", resolverErr: ErrIdentityMappingConflict, expected: ErrIdentityMappingConflict},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verifier := &verifierStub{identity: ExternalIdentity{ID: "kratos-id", Email: "person@example.com"}, err: test.verifierErr}
			resolver := &resolverStub{user: User{ID: "user-id", Status: StatusActive}, err: test.resolverErr}
			authenticator := NewAuthenticator(verifier, resolver)

			_, err := authenticator.Authenticate(context.Background(), Credentials{SessionToken: "opaque-token"})
			if !errors.Is(err, test.expected) {
				t.Fatalf("expected %v, got %v", test.expected, err)
			}
		})
	}
}
