package pipeline

import (
	"context"
	"fmt"
	"sync"

	"github.com/gass88wei/rgt-gsd/internal/auditor"
	"github.com/gass88wei/rgt-gsd/internal/executor"
	"github.com/gass88wei/rgt-gsd/internal/recovery"
	"github.com/gass88wei/rgt-gsd/internal/workspace"
)

// Pipeline orchestrates the four sub-agents for a complete run.
type Pipeline struct {
	ws  workspace.Workspace
	aud auditor.Auditor
	exec executor.Executor
	rec recovery.Recovery
}

// New creates a Pipeline with the given sub-agent implementations.
func New(ws workspace.Workspace, aud auditor.Auditor, exec executor.Executor, rec recovery.Recovery) *Pipeline {
	return &Pipeline{
		ws:  ws,
		aud: aud,
		exec: exec,
		rec: rec,
	}
}

// RunResult holds the outcome of a pipeline run.
type RunResult struct {
	SessionID string
	Steps     []auditor.Step
	Cost      executor.CostInfo
	Error     error
}

// Run executes the full pipeline: workspace check → record baseline → execute → record steps → recover on error.
func (p *Pipeline) Run(ctx context.Context, projectRoot string, plan executor.PlanSpec) (*RunResult, error) {
	// 1. Workspace: prepare environment
	if err := p.ws.Prepare(ctx); err != nil {
		return nil, fmt.Errorf("workspace prepare: %w", err)
	}

	// 2. Workspace: check clean
	clean, err := p.ws.IsClean(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace check: %w", err)
	}
	if !clean {
		return nil, workspace.ErrDirtyWorkspace
	}

	// 3. Workspace: get HEAD
	head, err := p.ws.CurrentHEAD(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace HEAD: %w", err)
	}

	// 4. Auditor: initialize
	if err := p.aud.Init(ctx, projectRoot); err != nil {
		return nil, fmt.Errorf("auditor init: %w", err)
	}

	// 5. Auditor: record baseline
	_, err = p.aud.Record(ctx, projectRoot, auditor.StepInput{
		SessionID: fmt.Sprintf("baseline-%s", head[:8]),
		Cause: auditor.Cause{
			ToolName: "rgt-gsd/run",
			ArgsJSON: fmt.Sprintf(`{"head": "%s"}`, head),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("auditor baseline: %w", err)
	}

	// 6. Executor: start gsd-2
	events, err := p.exec.Run(ctx, projectRoot, plan)
	if err != nil {
		return nil, fmt.Errorf("executor run: %w", err)
	}

	return p.processEvents(ctx, projectRoot, events)
}

func (p *Pipeline) processEvents(ctx context.Context, projectRoot string, events <-chan executor.ExecEvent) (*RunResult, error) {
	result := &RunResult{}
	var mu sync.Mutex
	retryCount := 0
	const maxRetries = 3

	for {
		select {
		case <-ctx.Done():
			return result, ctx.Err()

		case event, ok := <-events:
			if !ok {
				return result, nil
			}

			switch event.Type {
			case "unit_completed":
				hash, err := p.aud.Record(ctx, projectRoot, auditor.StepInput{
					SessionID: result.SessionID,
					Cause: auditor.Cause{
						ToolName: event.UnitType,
						ArgsJSON: fmt.Sprintf(`{"unit": "%s"}`, event.UnitID),
					},
				})
				if err != nil {
					return nil, fmt.Errorf("auditor record: %w", err)
				}
				mu.Lock()
				result.Steps = append(result.Steps, auditor.Step{
					Hash:      hash,
					SessionID: result.SessionID,
					Cause:     auditor.Cause{ToolName: event.UnitType},
				})
				result.Cost = event.Cost
				retryCount = 0
				mu.Unlock()

			case "blocked":
				return result, fmt.Errorf("gsd-2 blocked: %s", event.UnitID)

			case "error":
				retryCount++
				if retryCount > maxRetries {
					return result, fmt.Errorf("retry limit exceeded, %d consecutive failures", maxRetries)
				}

				failure := recovery.RecoverableFailure{
					SessionID:    result.SessionID,
					FailedUnit:   event.UnitID,
					ErrorType:    classifyError(event.Error),
					ErrorMessage: event.Error.Error(),
					CostSoFar:    result.Cost,
				}

				plan := p.rec.Analyze(ctx, failure)
				if plan.Action == "human" || plan.Action == "abort" {
					return result, fmt.Errorf("%w: %s", recovery.ErrUnrecoverable, plan.HumanNote)
				}

				resumeFrom, execErr := p.rec.Execute(ctx, plan, p.aud, projectRoot, result.SessionID)
				if execErr != nil {
					return result, fmt.Errorf("recovery execute: %w", execErr)
				}

				if resumeFrom == "" {
					return result, fmt.Errorf("recovery returned empty resume point")
				}

				newEvents, resumeErr := p.exec.Resume(ctx, result.SessionID, resumeFrom)
				if resumeErr != nil {
					return result, fmt.Errorf("executor resume: %w", resumeErr)
				}
				events = newEvents
				retryCount = 0

			case "cancelled":
				return result, nil

			case "completed":
				return result, nil
			}
		}
	}
}

func classifyError(err error) string {
	if err == nil {
		return "unknown"
	}
	msg := err.Error()
	switch {
	case containsFold(msg, "schema"), containsFold(msg, "unexpected tool"):
		return "tool_schema"
	case containsFold(msg, "policy"), containsFold(msg, "forbidden"):
		return "policy_block"
	case containsFold(msg, "worktree"), containsFold(msg, "invalid root"):
		return "worktree_invalid"
	case containsFold(msg, "rate limit"), containsFold(msg, "quota"):
		return "provider"
	case containsFold(msg, "network"), containsFold(msg, "connection"), containsFold(msg, "timeout"):
		return "network"
	case containsFold(msg, "deterministic"):
		return "deterministic"
	default:
		return "unknown"
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
