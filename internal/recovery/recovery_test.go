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
		FailedUnit: "task-3",
		FailedStep: "abc123",
	})

	if plan.Action != "retry" {
		t.Errorf("expected retry, got %s", plan.Action)
	}
	if plan.ResumeFrom != "task-3" {
		t.Errorf("expected task-3, got %s", plan.ResumeFrom)
	}
}

func TestAnalyzeNetwork(t *testing.T) {
	r := recovery.New()
	ctx := context.Background()

	plan := r.Analyze(ctx, recovery.RecoverableFailure{
		ErrorType:  "network",
		FailedUnit: "task-2",
	})

	if plan.Action != "retry" {
		t.Errorf("expected retry, got %s", plan.Action)
	}
}

func TestAnalyzeDeterministic(t *testing.T) {
	r := recovery.New()
	ctx := context.Background()

	plan := r.Analyze(ctx, recovery.RecoverableFailure{
		ErrorType:  "deterministic",
		FailedUnit: "task-5",
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

func TestExecuteRetry(t *testing.T) {
	r := recovery.New()
	ctx := context.Background()
	aud := auditor.New("rgt")

	plan := recovery.RecoveryPlan{
		Action:     "retry",
		ResumeFrom: "unit-42",
	}

	resumeFrom, err := r.Execute(ctx, plan, aud, "/tmp", "test-session")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if resumeFrom != "unit-42" {
		t.Errorf("expected unit-42, got %s", resumeFrom)
	}
}

func TestExecuteAbort(t *testing.T) {
	r := recovery.New()
	ctx := context.Background()
	aud := auditor.New("rgt")

	plan := recovery.RecoveryPlan{Action: "abort"}

	resumeFrom, err := r.Execute(ctx, plan, aud, "/tmp", "test-session")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if resumeFrom != "" {
		t.Errorf("expected empty resumeFrom for abort, got %s", resumeFrom)
	}
}

func TestExecuteHuman(t *testing.T) {
	r := recovery.New()
	ctx := context.Background()
	aud := auditor.New("rgt")

	plan := recovery.RecoveryPlan{Action: "human"}

	_, err := r.Execute(ctx, plan, aud, "/tmp", "test-session")
	if err != recovery.ErrUnrecoverable {
		t.Fatalf("expected ErrUnrecoverable, got %v", err)
	}
}

func TestExecuteRewindNoRGT(t *testing.T) {
	r := recovery.New()
	ctx := context.Background()
	aud := auditor.New("rgt")

	plan := recovery.RecoveryPlan{
		Action:     "rewind",
		RewindTo:   "abc123",
		ResumeFrom: "unit-10",
	}

	resumeFrom, err := r.Execute(ctx, plan, aud, "/tmp", "test-session")
	if err != nil {
		// rgt not installed, rewind fails — expected
		t.Logf("Execute rewind error (expected if rgt not installed): %v", err)
		return
	}
	if resumeFrom != "unit-10" {
		t.Errorf("expected unit-10, got %s", resumeFrom)
	}
}
