package kratos

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lupppig/credar/backend/internal/identity"
)

const testIdentityID = "0199c8f0-4afb-71e3-b5d7-97bac4f520a8"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestValidateSessionPrefersTokenAndMapsIdentity(t *testing.T) {
	client := newTestClient(t, "http://kratos.test", func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/sessions/whoami" {
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
		if request.Header.Get("Cookie") != "" {
			t.Fatalf("cookie must not be forwarded alongside an explicit token")
		}
		if request.Header.Get("X-Session-Token") != "token-value" {
			t.Fatalf("session token was not forwarded")
		}
		return response(http.StatusOK, `{
			"active": true,
			"identity": {
				"id": "`+testIdentityID+`",
				"state": "active",
				"traits": {"email": " Person@Example.COM "}
			}
		}`), nil
	})

	externalIdentity, err := client.ValidateSession(context.Background(), identity.Credentials{
		Cookie:       "ory_kratos_session=cookie-value",
		SessionToken: "token-value",
	})
	if err != nil {
		t.Fatalf("validate session: %v", err)
	}
	if externalIdentity.ID != testIdentityID || externalIdentity.Email != "person@example.com" {
		t.Fatalf("unexpected identity: %+v", externalIdentity)
	}
}

func TestValidateSessionForwardsCookieWhenTokenIsAbsent(t *testing.T) {
	client := newTestClient(t, "http://kratos.test", func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Cookie") != "ory_kratos_session=cookie-value" {
			t.Fatalf("cookie was not forwarded")
		}
		if request.Header.Get("X-Session-Token") != "" {
			t.Fatalf("unexpected session token")
		}
		return response(http.StatusOK, `{
			"active": true,
			"identity": {
				"id": "`+testIdentityID+`",
				"state": "active",
				"traits": {"email": "person@example.com"}
			}
		}`), nil
	})

	_, err := client.ValidateSession(context.Background(), identity.Credentials{
		Cookie: "ory_kratos_session=cookie-value",
	})
	if err != nil {
		t.Fatalf("validate cookie session: %v", err)
	}
}

func TestValidateSessionRejectsMissingOrExpiredSession(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := newTestClient(t, "http://kratos.test", func(*http.Request) (*http.Response, error) {
				return response(status, `{}`), nil
			})

			_, err := client.ValidateSession(context.Background(), identity.Credentials{SessionToken: "invalid"})
			if !errors.Is(err, identity.ErrInvalidSession) {
				t.Fatalf("expected invalid session, got %v", err)
			}
		})
	}
}

func TestValidateSessionRejectsInactiveSession(t *testing.T) {
	client := newTestClient(t, "http://kratos.test", func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, `{
			"active": false,
			"identity": {"id": "`+testIdentityID+`", "traits": {"email": "person@example.com"}}
		}`), nil
	})

	_, err := client.ValidateSession(context.Background(), identity.Credentials{SessionToken: "expired"})
	if !errors.Is(err, identity.ErrInvalidSession) {
		t.Fatalf("expected invalid session, got %v", err)
	}
}

func TestValidateSessionTreatsMalformedIdentityAsUnavailable(t *testing.T) {
	client := newTestClient(t, "http://kratos.test", func(*http.Request) (*http.Response, error) {
		return response(
			http.StatusOK,
			`{"active":true,"identity":{"id":"not-a-uuid","traits":{"email":"bad"}}}`,
		), nil
	})

	_, err := client.ValidateSession(context.Background(), identity.Credentials{SessionToken: "opaque"})
	if !errors.Is(err, identity.ErrAuthenticationUnavailable) {
		t.Fatalf("expected authentication unavailable, got %v", err)
	}
}

func TestCheckUsesKratosReadinessEndpoint(t *testing.T) {
	client := newTestClient(t, "http://kratos.test/kratos", func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/kratos/health/ready" {
			t.Fatalf("unexpected readiness path %q", request.URL.Path)
		}
		return response(http.StatusNoContent, ""), nil
	})

	if err := client.Check(context.Background()); err != nil {
		t.Fatalf("check: %v", err)
	}
}

func newTestClient(t *testing.T, publicURL string, roundTrip roundTripFunc) *Client {
	t.Helper()
	client, err := New(Config{
		PublicURL: publicURL,
		AdminURL:  publicURL,
		Timeout:   time.Second,
		HTTPClient: &http.Client{
			Transport: roundTrip,
		},
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
