package recovery

import (
	"context"

	"github.com/gass88wei/rgt-gsd/internal/auditor"
	"github.com/gass88wei/rgt-gsd/internal/executor"
)

// RecoverableFailure describes a gsd-2 execution failure.
type RecoverableFailure struct {
	SessionID    string
	FailedUnit   string
	FailedStep   string
	ErrorType    string // "tool_schema", "policy_block", "worktree_invalid", "provider", "network", "deterministic", "unknown"
	ErrorMessage string
	CostSoFar    executor.CostInfo
}

// RecoveryPlan describes how to recover from a failure.
type RecoveryPlan struct {
	Action     string // "rewind", "retry", "skip", "abort", "human"
	RewindTo   string // step hash to rewind to (if Action == "rewind")
	ResumeFrom string // unit to resume from
	Reason     string
	HumanNote  string // shown to user if Action == "human"
}

// Recovery analyzes failures and executes recovery plans.
type Recovery interface {
	// Analyze classifies a failure and produces a recovery plan.
	Analyze(ctx context.Context, failure RecoverableFailure) RecoveryPlan

	// Execute runs the recovery plan (rewind files, return resume point).
	// Returns the unit ID to resume from.
	Execute(ctx context.Context, plan RecoveryPlan, aud auditor.Auditor, projectRoot, sessionID string) (resumeFrom string, err error)
}
