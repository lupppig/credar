package kratos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lupppig/credar/backend/internal/identity"
)

const maxResponseBytes = 1 << 20

type Config struct {
	PublicURL  string
	AdminURL   string
	Timeout    time.Duration
	HTTPClient *http.Client
}

type Client struct {
	publicURL *url.URL
	adminURL  *url.URL
	http      *http.Client
}

func New(config Config) (*Client, error) {
	publicURL, err := url.Parse(config.PublicURL)
	if err != nil {
		return nil, fmt.Errorf("parse Kratos public URL: %w", err)
	}
	if publicURL.Scheme != "http" && publicURL.Scheme != "https" {
		return nil, fmt.Errorf("Kratos public URL scheme must be http or https")
	}
	if publicURL.Host == "" || publicURL.User != nil || publicURL.RawQuery != "" || publicURL.Fragment != "" {
		return nil, fmt.Errorf("Kratos public URL must contain only a scheme, host, and optional path")
	}
	adminURL, err := url.Parse(config.AdminURL)
	if err != nil {
		return nil, fmt.Errorf("parse Kratos admin URL: %w", err)
	}
	if adminURL.Scheme != "http" && adminURL.Scheme != "https" {
		return nil, fmt.Errorf("Kratos admin URL scheme must be http or https")
	}
	if adminURL.Host == "" || adminURL.User != nil || adminURL.RawQuery != "" || adminURL.Fragment != "" {
		return nil, fmt.Errorf("Kratos admin URL must contain only a scheme, host, and optional path")
	}

	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: config.Timeout}
	}

	return &Client{publicURL: publicURL, adminURL: adminURL, http: client}, nil
}

func (client *Client) Name() string {
	return "kratos"
}

func (client *Client) Start(context.Context) error {
	return nil
}

func (client *Client) Check(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint(client.adminURL, "health/ready"), nil)
	if err != nil {
		return fmt.Errorf("create Kratos readiness request: %w", err)
	}

	response, err := client.http.Do(request)
	if err != nil {
		return fmt.Errorf("check Kratos readiness: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("check Kratos readiness: unexpected status %d", response.StatusCode)
	}
	return nil
}

func (client *Client) Close(context.Context) error {
	return nil
}

func (client *Client) ValidateSession(
	ctx context.Context,
	credentials identity.Credentials,
) (identity.ExternalIdentity, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint(client.publicURL, "sessions/whoami"), nil)
	if err != nil {
		return identity.ExternalIdentity{}, fmt.Errorf("create Kratos session request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if credentials.SessionToken != "" {
		request.Header.Set("X-Session-Token", credentials.SessionToken)
	} else if credentials.Cookie != "" {
		request.Header.Set("Cookie", credentials.Cookie)
	}

	response, err := client.http.Do(request)
	if err != nil {
		return identity.ExternalIdentity{}, errors.Join(identity.ErrAuthenticationUnavailable, err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized ||
		response.StatusCode == http.StatusForbidden ||
		response.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		return identity.ExternalIdentity{}, identity.ErrInvalidSession
	}
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		return identity.ExternalIdentity{}, fmt.Errorf(
			"%w: Kratos returned status %d",
			identity.ErrAuthenticationUnavailable,
			response.StatusCode,
		)
	}

	var session struct {
		Active   bool `json:"active"`
		Identity struct {
			ID     string `json:"id"`
			State  string `json:"state"`
			Traits struct {
				Email string `json:"email"`
			} `json:"traits"`
		} `json:"identity"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	if err := decoder.Decode(&session); err != nil {
		return identity.ExternalIdentity{}, errors.Join(identity.ErrAuthenticationUnavailable, err)
	}

	if !session.Active || (session.Identity.State != "" && session.Identity.State != "active") {
		return identity.ExternalIdentity{}, identity.ErrInvalidSession
	}
	if _, err := uuid.Parse(session.Identity.ID); err != nil {
		return identity.ExternalIdentity{}, fmt.Errorf("%w: Kratos identity ID is invalid", identity.ErrAuthenticationUnavailable)
	}

	email, err := normalizeEmail(session.Identity.Traits.Email)
	if err != nil {
		return identity.ExternalIdentity{}, fmt.Errorf("%w: Kratos identity email is invalid", identity.ErrAuthenticationUnavailable)
	}

	return identity.ExternalIdentity{ID: strings.ToLower(session.Identity.ID), Email: email}, nil
}

// LookupIdentity resolves one identity through the Kratos admin API for
// explicit operator-controlled bootstrap flows.
func (client *Client) LookupIdentity(ctx context.Context, identityID string) (identity.ExternalIdentity, error) {
	if _, err := uuid.Parse(identityID); err != nil {
		return identity.ExternalIdentity{}, fmt.Errorf("invalid Kratos identity ID")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint(client.adminURL, "admin/identities/"+identityID), nil)
	if err != nil {
		return identity.ExternalIdentity{}, fmt.Errorf("create Kratos identity request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.http.Do(request)
	if err != nil {
		return identity.ExternalIdentity{}, errors.Join(identity.ErrAuthenticationUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		return identity.ExternalIdentity{}, identity.ErrInvalidSession
	}
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		return identity.ExternalIdentity{}, fmt.Errorf("%w: Kratos returned status %d", identity.ErrAuthenticationUnavailable, response.StatusCode)
	}
	var payload struct {
		ID     string `json:"id"`
		State  string `json:"state"`
		Traits struct {
			Email string `json:"email"`
		} `json:"traits"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&payload); err != nil {
		return identity.ExternalIdentity{}, errors.Join(identity.ErrAuthenticationUnavailable, err)
	}
	if payload.State != "" && payload.State != "active" {
		return identity.ExternalIdentity{}, identity.ErrInvalidSession
	}
	email, err := normalizeEmail(payload.Traits.Email)
	if err != nil {
		return identity.ExternalIdentity{}, errors.Join(identity.ErrAuthenticationUnavailable, err)
	}
	return identity.ExternalIdentity{ID: strings.ToLower(payload.ID), Email: email}, nil
}

func endpoint(baseURL *url.URL, relativePath string) string {
	base := *baseURL
	base.Path = strings.TrimSuffix(base.Path, "/") + "/"
	return base.ResolveReference(&url.URL{Path: relativePath}).String()
}

func normalizeEmail(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(normalized)
	if err != nil || parsed.Address != normalized {
		return "", errors.New("invalid email address")
	}
	return normalized, nil
}
