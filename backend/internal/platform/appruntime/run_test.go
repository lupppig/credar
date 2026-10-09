package appruntime_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lupppig/credar/backend/internal/platform/appruntime"
)

func TestRunStartsAndClosesDependenciesOnCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	dependencies := newFakeDependencies()
	server := &http.Server{
		Addr:              "127.0.0.1:0",
		Handler:           http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		ReadHeaderTimeout: time.Second,
	}

	result := make(chan error, 1)
	go func() {
		result <- appruntime.Run(ctx, server, dependencies, time.Second, discardLogger())
	}()

	select {
	case <-dependencies.started:
	case <-time.After(time.Second):
		t.Fatal("dependencies did not start")
	}
	cancel()

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("run application: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("application did not stop after cancellation")
	}

	if !dependencies.wasClosed() {
		t.Fatal("expected dependencies to close")
	}
}

func TestRunCleansUpAfterDependencyStartupFailure(t *testing.T) {
	t.Parallel()

	dependencies := newFakeDependencies()
	dependencies.startErr = errors.New("cannot connect")
	server := &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}

	err := appruntime.Run(context.Background(), server, dependencies, time.Second, discardLogger())
	if err == nil || !strings.Contains(err.Error(), "cannot connect") {
		t.Fatalf("expected startup error, got %v", err)
	}
	if !dependencies.wasClosed() {
		t.Fatal("expected dependencies to close after startup failure")
	}
}

type fakeDependencies struct {
	started  chan struct{}
	startErr error

	mu     sync.Mutex
	closed bool
}

func newFakeDependencies() *fakeDependencies {
	return &fakeDependencies{started: make(chan struct{})}
}

func (dependencies *fakeDependencies) Start(context.Context) error {
	close(dependencies.started)
	return dependencies.startErr
}

func (dependencies *fakeDependencies) Close(context.Context) error {
	dependencies.mu.Lock()
	defer dependencies.mu.Unlock()
	dependencies.closed = true
	return nil
}

func (dependencies *fakeDependencies) wasClosed() bool {
	dependencies.mu.Lock()
	defer dependencies.mu.Unlock()
	return dependencies.closed
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
