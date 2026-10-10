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

func performRequest(authenticator Authenticator, cookie string, token string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	New(authenticator).RegisterRoutes(router)

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
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}
