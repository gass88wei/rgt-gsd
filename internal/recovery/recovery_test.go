package recovery_test

import (
	"context"
	"testing"

	"github.com/gass88wei/rgt-gsd/internal/auditor"
	"github.com/gass88wei/rgt-gsd/internal/recovery"
)

func TestNew(t *testing.T) {
	r := recovery.New()
	if r == nil {
		t.Fatal("New returned nil")
	}
}

func TestAnalyzeToolSchema(t *testing.T) {
	r := recovery.New()
	ctx := context.Background()

	plan := r.Analyze(ctx, recovery.RecoverableFailure{
		ErrorType:  "tool_schema",
		FailedStep: "abc123",
	})

	if plan.Action != "retry" {
		t.Errorf("expected retry, got %s", plan.Action)
	}
}

func TestAnalyzeNetwork(t *testing.T) {
	r := recovery.New()
	ctx := context.Background()

	plan := r.Analyze(ctx, recovery.RecoverableFailure{
		ErrorType: "network",
	})

	if plan.Action != "retry" {
		t.Errorf("expected retry, got %s", plan.Action)
	}
}

func TestAnalyzeDeterministic(t *testing.T) {
	r := recovery.New()
	ctx := context.Background()

	plan := r.Analyze(ctx, recovery.RecoverableFailure{
		ErrorType: "deterministic",
	})

	if plan.Action != "abort" {
		t.Errorf("expected abort, got %s", plan.Action)
	}
}

func TestAnalyzeProviderRateLimit(t *testing.T) {
	r := recovery.New()
	ctx := context.Background()

	plan := r.Analyze(ctx, recovery.RecoverableFailure{
		ErrorType:    "provider",
		ErrorMessage: "rate limit exceeded",
	})

	if plan.Action != "abort" {
		t.Errorf("expected abort, got %s", plan.Action)
	}
}

func TestAnalyzeUnknown(t *testing.T) {
	r := recovery.New()
	ctx := context.Background()

	plan := r.Analyze(ctx, recovery.RecoverableFailure{
		ErrorType: "unknown",
	})

	if plan.Action != "human" {
		t.Errorf("expected human, got %s", plan.Action)
	}
}

func TestAnalyzeEmpty(t *testing.T) {
	r := recovery.New()
	ctx := context.Background()

	plan := r.Analyze(ctx, recovery.RecoverableFailure{})

	if plan.Action != "human" {
		t.Errorf("expected human for empty failure, got %s", plan.Action)
	}
}

func TestExecuteRewindNonRewind(t *testing.T) {
	r := recovery.New()
	ctx := context.Background()
	aud := auditor.New("rgt")

	plan := recovery.RecoveryPlan{Action: "retry"}

	err := r.ExecuteRewind(ctx, plan, aud, "/tmp")
	if err != nil {
		t.Fatalf("ExecuteRewind should be nil for non-rewind plans: %v", err)
	}
}

func TestExecuteRewindEmptyHash(t *testing.T) {
	r := recovery.New()
	ctx := context.Background()
	aud := auditor.New("rgt")

	plan := recovery.RecoveryPlan{Action: "rewind", RewindTo: ""}

	err := r.ExecuteRewind(ctx, plan, aud, "/tmp")
	if err != nil {
		t.Fatalf("ExecuteRewind should be nil for empty hash: %v", err)
	}
}
