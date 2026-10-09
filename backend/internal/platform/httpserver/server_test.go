package httpserver_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lupppig/credar/backend/internal/platform/dependency"
	platformhttp "github.com/lupppig/credar/backend/internal/platform/httpserver"
	"github.com/lupppig/credar/backend/internal/platform/identifier"
	"github.com/lupppig/credar/backend/internal/platform/requestcontext"
)

func TestHealthEndpointReportsProcessHealth(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := platformhttp.New(":0", logger, readyDependencies{})
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	application.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	contentType, _, err := mime.ParseMediaType(response.Header().Get("Content-Type"))
	if err != nil {
		t.Fatalf("parse response content type: %v", err)
	}
	if contentType != "application/json" {
		t.Fatalf("expected application/json content type, got %q", contentType)
	}
	requestID := response.Header().Get("X-Request-ID")
	if !identifier.IsUUIDv7(requestID) {
		t.Fatalf("expected a UUIDv7 request ID, got %q", requestID)
	}
	correlationID := response.Header().Get("X-Correlation-ID")
	if !identifier.IsUUIDv7(correlationID) {
		t.Fatalf("expected a UUIDv7 correlation ID, got %q", correlationID)
	}
	if correlationID != requestID {
		t.Fatalf("expected a missing correlation ID to use request ID %q, got %q", requestID, correlationID)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("expected ok health status, got %q", body.Status)
	}
}

func TestHealthEndpointRejectsUnsupportedMethods(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := platformhttp.New(":0", logger, readyDependencies{})
	request := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	response := httptest.NewRecorder()

	application.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, response.Code)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("expected Allow header %q, got %q", http.MethodGet, allow)
	}
}

func TestHealthEndpointPreservesValidClientIdentifiers(t *testing.T) {
	t.Parallel()

	const requestID = "01991a4b-fa00-7000-8000-000000000001"
	const correlationID = "01991a4b-fa00-7000-8000-000000000002"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := platformhttp.New(":0", logger, readyDependencies{})
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("X-Request-ID", requestID)
	request.Header.Set("X-Correlation-ID", correlationID)
	response := httptest.NewRecorder()

	application.Handler.ServeHTTP(response, request)

	if received := response.Header().Get("X-Request-ID"); received != requestID {
		t.Fatalf("expected request ID %q, got %q", requestID, received)
	}
	if received := response.Header().Get("X-Correlation-ID"); received != correlationID {
		t.Fatalf("expected correlation ID %q, got %q", correlationID, received)
	}
}

func TestRequestIdentifiersPropagateOnStandardContext(t *testing.T) {
	t.Parallel()

	const requestID = "01991a4b-fa00-7000-8000-000000000001"
	const correlationID = "01991a4b-fa00-7000-8000-000000000002"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := platformhttp.New(":0", logger, readyDependencies{})
	router, ok := application.Handler.(*gin.Engine)
	if !ok {
		t.Fatal("expected Gin engine handler")
	}
	router.GET("/context-identifiers", func(context *gin.Context) {
		context.JSON(http.StatusOK, gin.H{
			"request_id":     requestcontext.RequestID(context.Request.Context()),
			"correlation_id": requestcontext.CorrelationID(context.Request.Context()),
		})
	})

	request := httptest.NewRequest(http.MethodGet, "/context-identifiers", nil)
	request.Header.Set("X-Request-ID", requestID)
	request.Header.Set("X-Correlation-ID", correlationID)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)

	var body map[string]string
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["request_id"] != requestID || body["correlation_id"] != correlationID {
		t.Fatalf("identifiers did not propagate: %#v", body)
	}
}

func TestReadinessEndpointReflectsDependencies(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	readiness := &controlledReadiness{ready: false}
	application := platformhttp.New(":0", logger, readiness)

	unavailableRequest := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	unavailableResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(unavailableResponse, unavailableRequest)
	if unavailableResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unavailable status, got %d", unavailableResponse.Code)
	}

	readiness.ready = true
	readyRequest := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	readyResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(readyResponse, readyRequest)
	if readyResponse.Code != http.StatusOK {
		t.Fatalf("expected ready status, got %d", readyResponse.Code)
	}
}

type readyDependencies struct{}

func (readyDependencies) Readiness(context.Context) dependency.ReadinessReport {
	return dependency.ReadinessReport{Ready: true}
}

type controlledReadiness struct {
	ready bool
}

func (readiness *controlledReadiness) Readiness(context.Context) dependency.ReadinessReport {
	return dependency.ReadinessReport{
		Ready:  readiness.ready,
		Checks: []dependency.CheckResult{{Name: "postgres", Ready: readiness.ready}},
	}
}
