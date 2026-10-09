package dependency_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/lupppig/credar/backend/internal/platform/dependency"
)

func TestSetStartsInOrderAndClosesInReverse(t *testing.T) {
	t.Parallel()

	var events eventLog
	first := &fakeComponent{name: "first", events: &events}
	second := &fakeComponent{name: "second", events: &events}
	set, err := dependency.NewSet(time.Second, first, second)
	if err != nil {
		t.Fatalf("construct dependencies: %v", err)
	}

	if err := set.Start(context.Background()); err != nil {
		t.Fatalf("start dependencies: %v", err)
	}
	if err := set.Close(context.Background()); err != nil {
		t.Fatalf("close dependencies: %v", err)
	}

	want := []string{"start:first", "start:second", "close:second", "close:first"}
	if got := events.values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected lifecycle order: got %v, want %v", got, want)
	}
}

func TestSetRetainsStartedComponentsAfterPartialFailureForCleanup(t *testing.T) {
	t.Parallel()

	var events eventLog
	first := &fakeComponent{name: "first", events: &events}
	second := &fakeComponent{name: "second", events: &events, startErr: errors.New("unavailable")}
	set, err := dependency.NewSet(time.Second, first, second)
	if err != nil {
		t.Fatalf("construct dependencies: %v", err)
	}

	if err := set.Start(context.Background()); err == nil {
		t.Fatal("expected startup failure")
	}
	if err := set.Close(context.Background()); err != nil {
		t.Fatalf("close partially started dependencies: %v", err)
	}

	want := []string{"start:first", "start:second", "close:first"}
	if got := events.values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected lifecycle order: got %v, want %v", got, want)
	}
}

func TestReadinessChangesWithDependencyHealth(t *testing.T) {
	t.Parallel()

	component := &fakeComponent{name: "postgres", events: &eventLog{}}
	set, err := dependency.NewSet(time.Second, component)
	if err != nil {
		t.Fatalf("construct dependencies: %v", err)
	}
	if err := set.Start(context.Background()); err != nil {
		t.Fatalf("start dependencies: %v", err)
	}

	if report := set.Readiness(context.Background()); !report.Ready {
		t.Fatalf("expected healthy dependency set, got %#v", report)
	}

	component.setCheckError(errors.New("connection lost"))
	if report := set.Readiness(context.Background()); report.Ready {
		t.Fatalf("expected unhealthy dependency set, got %#v", report)
	}
}

func TestEmptySetBecomesReadyOnlyAfterStartup(t *testing.T) {
	t.Parallel()

	set, err := dependency.NewSet(time.Second)
	if err != nil {
		t.Fatalf("construct dependencies: %v", err)
	}
	if report := set.Readiness(context.Background()); report.Ready {
		t.Fatal("expected dependency set to be unavailable before startup")
	}
	if err := set.Start(context.Background()); err != nil {
		t.Fatalf("start dependencies: %v", err)
	}
	if report := set.Readiness(context.Background()); !report.Ready {
		t.Fatal("expected empty dependency set to be ready after startup")
	}
	if err := set.Start(context.Background()); err == nil {
		t.Fatal("expected duplicate startup to fail")
	}
}

type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (log *eventLog) add(event string) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.events = append(log.events, event)
}

func (log *eventLog) values() []string {
	log.mu.Lock()
	defer log.mu.Unlock()
	return append([]string(nil), log.events...)
}

type fakeComponent struct {
	name     string
	events   *eventLog
	startErr error

	mu       sync.RWMutex
	checkErr error
}

func (component *fakeComponent) Name() string {
	return component.name
}

func (component *fakeComponent) Start(context.Context) error {
	component.events.add("start:" + component.name)
	return component.startErr
}

func (component *fakeComponent) Check(context.Context) error {
	component.mu.RLock()
	defer component.mu.RUnlock()
	return component.checkErr
}

func (component *fakeComponent) Close(context.Context) error {
	component.events.add("close:" + component.name)
	return nil
}

func (component *fakeComponent) setCheckError(err error) {
	component.mu.Lock()
	defer component.mu.Unlock()
	component.checkErr = err
}
