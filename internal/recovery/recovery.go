package recovery

import (
	"context"

	"github.com/gass88wei/rgt-gsd/internal/auditor"
)

type defaultRecovery struct{}

// New creates a default Recovery implementation.
func New() Recovery {
	return &defaultRecovery{}
}

func (r *defaultRecovery) Analyze(ctx context.Context, failure RecoverableFailure) RecoveryPlan {
	switch failure.ErrorType {
	case "tool_schema":
		return RecoveryPlan{
			Action:     "retry",
			ResumeFrom: failure.FailedUnit,
			Reason:    "tool schema mismatch, retry may succeed with corrected schema",
		}

	case "policy_block":
		return RecoveryPlan{
			Action:     "rewind",
			RewindTo:   failure.FailedStep,
			ResumeFrom: failure.FailedUnit,
			Reason:    "policy blocked execution; rewinding to re-attempt with adjusted policy",
		}

	case "worktree_invalid":
		return RecoveryPlan{
			Action:     "rewind",
			ResumeFrom: failure.FailedUnit,
			Reason:    "worktree is invalid; rewinding to rebuild worktree state",
		}

	case "network":
		return RecoveryPlan{
			Action:     "retry",
			ResumeFrom: failure.FailedUnit,
			Reason:    "transient network error, retry expected to succeed",
		}

	case "provider":
		if containsFold(failure.ErrorMessage, "rate limit") || containsFold(failure.ErrorMessage, "quota") {
			return RecoveryPlan{
				Action:    "abort",
				Reason:    "provider rate limit or quota exhausted",
				HumanNote: "Provider quota exhausted. Wait for quota reset then run 'rgt-gsd recover' to resume.",
			}
		}
		return RecoveryPlan{
			Action:     "retry",
			ResumeFrom: failure.FailedUnit,
			Reason:    "provider error, retry may succeed",
		}

	case "deterministic":
		return RecoveryPlan{
			Action:    "abort",
			Reason:    "deterministic failure, retry will not help",
			HumanNote: "This is a deterministic failure. Check the error message and fix the underlying issue before resuming.",
		}

	default:
		return RecoveryPlan{
			Action:    "human",
			Reason:    "unknown error type, human review needed",
			HumanNote: "GSd-2 encountered an unexpected error. Review the error and decide whether to retry, rewind, or skip.",
		}
	}
}

func (r *defaultRecovery) Execute(ctx context.Context, plan RecoveryPlan, aud auditor.Auditor, projectRoot, sessionID string) (string, error) {
	switch plan.Action {
	case "rewind":
		if plan.RewindTo != "" {
			if err := aud.Rewind(ctx, projectRoot, plan.RewindTo); err != nil {
				return "", ErrRewindFailed
			}
		}
		return plan.ResumeFrom, nil

	case "retry":
		return plan.ResumeFrom, nil

	case "skip":
		return plan.ResumeFrom, nil

	case "abort":
		return "", nil

	case "human":
		return "", ErrUnrecoverable

	default:
		return "", ErrUnrecoverable
	}
}

func containsFold(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			ca, cb := s[i+j], substr[j]
			if ca >= 'A' && ca <= 'Z' {
				ca += 32
			}
			if cb >= 'A' && cb <= 'Z' {
				cb += 32
			}
			if ca != cb {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
