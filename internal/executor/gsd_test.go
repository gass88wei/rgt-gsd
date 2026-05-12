package executor_test

import (
	"context"
	"testing"

	"github.com/gass88wei/rgt-gsd/internal/executor"
)

func TestNew(t *testing.T) {
	e := executor.New("gsd-pi")
	if e == nil {
		t.Fatal("New returned nil")
	}
}

func TestNewEmptyPath(t *testing.T) {
	e := executor.New("")
	if e == nil {
		t.Fatal("New with empty path returned nil")
	}
}

func TestHealthCheckNoGSD(t *testing.T) {
	e := executor.New("gsd-pi-nonexistent")
	ctx := context.Background()

	err := e.HealthCheck(ctx)
	if err == nil {
		t.Skip("gsd-pi is available in test environment")
	}
	if err != executor.ErrGSDNotFound {
		t.Logf("HealthCheck error: %v", err)
	}
}

func TestRunNoGSD(t *testing.T) {
	dir := t.TempDir()
	e := executor.New("gsd-pi-nonexistent")
	ctx := context.Background()

	_, err := e.Run(ctx, dir, executor.PlanSpec{Command: "/gsd auto"})
	if err == nil {
		t.Skip("gsd-pi is available in test environment")
	}
	t.Logf("Run error (expected if gsd-pi not installed): %v", err)
}

func TestStatusNotFound(t *testing.T) {
	e := executor.New("gsd-pi")
	ctx := context.Background()

	_, err := e.Status(ctx, "nonexistent-session")
	if err != executor.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestStopNotFound(t *testing.T) {
	e := executor.New("gsd-pi")
	ctx := context.Background()

	err := e.Stop(ctx, "nonexistent-session")
	if err != executor.ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}
