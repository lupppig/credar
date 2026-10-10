package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/lupppig/credar/backend/internal/identity"
	"github.com/lupppig/credar/backend/internal/platform/httpproblem"
	"github.com/lupppig/credar/backend/internal/platform/requestcontext"
)

const (
	testRequestID     = "0199c8f0-4afb-71e3-b5d7-97bac4f520a8"
	testCorrelationID = "0199c8f0-4afb-71e3-b5d7-97bac4f520a9"
)

type authenticatorStub struct {
	user        identity.User
	err         error
	credentials identity.Credentials
}

type sessionResolverStub struct {
	memberships      []identity.MembershipSummary
	activeTenantID   *string
	err              error
	userID           string
	selectedTenantID string
}

func (stub *sessionResolverStub) ResolveSessionContext(
	_ context.Context,
	userID string,
	selectedTenantID string,
) ([]identity.MembershipSummary, *string, error) {
	stub.userID = userID
	stub.selectedTenantID = selectedTenantID
	return stub.memberships, stub.activeTenantID, stub.err
}

func (stub *authenticatorStub) Authenticate(
	_ context.Context,
	credentials identity.Credentials,
) (identity.User, error) {
	stub.credentials = credentials
	return stub.user, stub.err
}

func TestSessionContextReturnsMappedUser(t *testing.T) {
	createdAt := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	authenticator := &authenticatorStub{user: identity.User{
		ID:        testRequestID,
		Email:     "person@example.com",
		Status:    identity.StatusActive,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}}
	recorder := performRequest(authenticator, "ory_kratos_session=cookie", "token")

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if authenticator.credentials.Cookie != "ory_kratos_session=cookie" || authenticator.credentials.SessionToken != "token" {
		t.Fatalf("credentials were not forwarded: %+v", authenticator.credentials)
	}
	var body struct {
		User           identity.User `json:"user"`
		Memberships    []any         `json:"memberships"`
		ActiveTenantID *string       `json:"active_tenant_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.User.ID != testRequestID || body.Memberships == nil || len(body.Memberships) != 0 || body.ActiveTenantID != nil {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestSessionContextMapsAuthenticationErrorsToProblems(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "missing", err: identity.ErrAuthenticationRequired, status: 401, code: "authentication_required"},
		{name: "invalid", err: identity.ErrInvalidSession, status: 401, code: "invalid_session"},
		{name: "disabled", err: identity.ErrUserDisabled, status: 403, code: "user_disabled"},
		{name: "conflict", err: identity.ErrIdentityMappingConflict, status: 409, code: "identity_mapping_conflict"},
		{name: "unavailable", err: identity.ErrAuthenticationUnavailable, status: 503, code: "authentication_unavailable"},
		{name: "unexpected", err: errors.New("unexpected"), status: 500, code: "internal_server_error"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := performRequest(&authenticatorStub{err: test.err}, "", "")
			if recorder.Code != test.status {
				t.Fatalf("expected %d, got %d: %s", test.status, recorder.Code, recorder.Body.String())
			}
			if !strings.HasPrefix(recorder.Header().Get("Content-Type"), httpproblem.ContentType) {
				t.Fatalf("unexpected content type %q", recorder.Header().Get("Content-Type"))
			}
			var problem httpproblem.Problem
			if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			if problem.Code != test.code || problem.RequestID != testRequestID || problem.CorrelationID != testCorrelationID {
				t.Fatalf("unexpected problem: %+v", problem)
			}
		})
	}
}

func TestSessionContextReturnsMembershipsAndSelectedTenant(t *testing.T) {
	selectedTenantID := testCorrelationID
	authenticator := &authenticatorStub{user: identity.User{
		ID:     testRequestID,
		Status: identity.StatusActive,
	}}
	resolver := &sessionResolverStub{
		memberships: []identity.MembershipSummary{{
			ID: testRequestID,
			Tenant: identity.TenantSummary{
				ID:     selectedTenantID,
				Name:   "Northstar Finance",
				Status: "ACTIVE",
			},
			Status:    "ACTIVE",
			RoleNames: []string{},
		}},
		activeTenantID: &selectedTenantID,
	}
	recorder := performRequestWithResolver(
		authenticator,
		resolver,
		"",
		"token",
		selectedTenantID,
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if resolver.userID != testRequestID || resolver.selectedTenantID != selectedTenantID {
		t.Fatalf("unexpected resolver input: user=%q tenant=%q", resolver.userID, resolver.selectedTenantID)
	}
	var body struct {
		Memberships    []identity.MembershipSummary `json:"memberships"`
		ActiveTenantID *string                      `json:"active_tenant_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Memberships) != 1 || body.ActiveTenantID == nil || *body.ActiveTenantID != selectedTenantID {
		t.Fatalf("unexpected session context: %+v", body)
	}
}

func TestSessionContextMapsTenantResolutionErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "malformed", err: identity.ErrInvalidTenantContext, status: 400, code: "invalid_tenant_context"},
		{name: "inactive", err: identity.ErrMembershipInactive, status: 403, code: "membership_inactive"},
		{name: "unavailable", err: identity.ErrTenantContextUnavailable, status: 503, code: "tenant_context_unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authenticator := &authenticatorStub{user: identity.User{ID: testRequestID, Status: identity.StatusActive}}
			recorder := performRequestWithResolver(
				authenticator,
				&sessionResolverStub{err: test.err},
				"",
				"token",
				testCorrelationID,
			)
			if recorder.Code != test.status {
				t.Fatalf("expected %d, got %d: %s", test.status, recorder.Code, recorder.Body.String())
			}
			var problem httpproblem.Problem
			if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			if problem.Code != test.code {
				t.Fatalf("expected code %q, got %q", test.code, problem.Code)
			}
		})
	}
}

func performRequest(authenticator Authenticator, cookie string, token string) *httptest.ResponseRecorder {
	return performRequestWithResolver(authenticator, nil, cookie, token, "")
}

func performRequestWithResolver(
	authenticator Authenticator,
	resolver identity.SessionContextResolver,
	cookie string,
	token string,
	tenantID string,
) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if resolver == nil {
		New(authenticator).RegisterRoutes(router)
	} else {
		New(authenticator, resolver).RegisterRoutes(router)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/session/context", nil)
	request = request.WithContext(requestcontext.WithIdentifiers(
		request.Context(),
		testRequestID,
		testCorrelationID,
	))
	if cookie != "" {
		request.Header.Set("Cookie", cookie)
	}
	if token != "" {
		request.Header.Set("X-Session-Token", token)
	}
	if tenantID != "" {
		request.Header.Set("X-Tenant-ID", tenantID)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}
