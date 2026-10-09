package healthcheck_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/lupppig/credar/backend/internal/platform/healthcheck"
)

func TestCheckAcceptsReadyResponse(t *testing.T) {
	t.Parallel()
	client := stubClient{status: http.StatusOK}

	if err := healthcheck.Check(context.Background(), client, "http://backend/readyz"); err != nil {
		t.Fatalf("expected readiness check to pass: %v", err)
	}
}

func TestCheckRejectsUnavailableResponse(t *testing.T) {
	t.Parallel()
	client := stubClient{status: http.StatusServiceUnavailable}

	if err := healthcheck.Check(context.Background(), client, "http://backend/readyz"); err == nil {
		t.Fatal("expected unavailable readiness check to fail")
	}
}

type stubClient struct {
	status int
}

func (client stubClient) Do(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: client.status,
		Body:       io.NopCloser(strings.NewReader("{}")),
	}, nil
}
