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
			Action: "retry",
			Reason: "tool schema mismatch, retry may succeed with corrected schema",
		}

	case "policy_block":
		return RecoveryPlan{
			Action:   "rewind",
			RewindTo: failure.FailedStep,
			Reason:   "policy blocked execution; rewinding to re-attempt with adjusted policy",
		}

	case "worktree_invalid":
		return RecoveryPlan{
			Action: "rewind",
			Reason: "worktree is invalid; rewinding to rebuild worktree state",
		}

	case "network":
		return RecoveryPlan{
			Action: "retry",
			Reason: "transient network error, retry expected to succeed",
		}

	case "provider":
		if containsFold(failure.ErrorMessage, "rate limit") || containsFold(failure.ErrorMessage, "quota") {
			return RecoveryPlan{
				Action:    "abort",
				Reason:    "provider rate limit or quota exhausted",
				HumanNote: "Provider quota exhausted. Wait for quota reset then retry.",
			}
		}
		return RecoveryPlan{
			Action: "retry",
			Reason: "provider error, retry may succeed",
		}

	case "deterministic":
		return RecoveryPlan{
			Action:    "abort",
			Reason:    "deterministic failure, retry will not help",
			HumanNote: "This is a deterministic failure. Fix the underlying issue before retrying.",
		}

	default:
		return RecoveryPlan{
			Action:    "human",
			Reason:    "unknown error type, human review needed",
			HumanNote: "Unexpected error. Review and decide whether to retry, rewind, or skip.",
		}
	}
}

func (r *defaultRecovery) ExecuteRewind(ctx context.Context, plan RecoveryPlan, aud auditor.Auditor, projectRoot string) error {
	if plan.Action != "rewind" || plan.RewindTo == "" {
		return nil
	}
	if err := aud.Rewind(ctx, projectRoot, plan.RewindTo); err != nil {
		return ErrRewindFailed
	}
	return nil
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
