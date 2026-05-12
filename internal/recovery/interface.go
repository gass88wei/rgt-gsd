package recovery

import (
	"context"

	"github.com/gass88wei/rgt-gsd/internal/auditor"
)

// RecoverableFailure describes an agent execution failure.
type RecoverableFailure struct {
	FailedStep   string // re_gent step hash
	ErrorType    string // "tool_schema", "policy_block", "worktree_invalid", "provider", "network", "deterministic", "unknown"
	ErrorMessage string
}

// RecoveryPlan describes how to recover from a failure.
type RecoveryPlan struct {
	Action    string // "rewind", "retry", "skip", "abort", "human"
	RewindTo  string // step hash to rewind to (if Action == "rewind")
	Reason    string
	HumanNote string // shown to user if Action == "human"
}

// Recovery analyzes failures and suggests recovery actions.
// It does NOT execute the plan — the caller decides what to do.
type Recovery interface {
	// Analyze classifies a failure and produces a recovery plan.
	Analyze(ctx context.Context, failure RecoverableFailure) RecoveryPlan

	// Execute runs only the rewind portion of a plan (restore files).
	// Returns nil on success. Caller handles retry/resume logic.
	ExecuteRewind(ctx context.Context, plan RecoveryPlan, aud auditor.Auditor, projectRoot string) error
}
