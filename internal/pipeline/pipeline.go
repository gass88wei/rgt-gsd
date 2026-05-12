package pipeline

import (
	"context"
	"fmt"

	"github.com/gass88wei/rgt-gsd/internal/auditor"
	"github.com/gass88wei/rgt-gsd/internal/recovery"
	"github.com/gass88wei/rgt-gsd/internal/workspace"
)

// Pipeline orchestrates workspace + audit + recovery sub-agents.
type Pipeline struct {
	ws  workspace.Workspace
	aud auditor.Auditor
	rec recovery.Recovery
}

// New creates a Pipeline with the given sub-agent implementations.
func New(ws workspace.Workspace, aud auditor.Auditor, rec recovery.Recovery) *Pipeline {
	return &Pipeline{ws: ws, aud: aud, rec: rec}
}

// Init prepares a project: workspace dirs + rgt init.
func (p *Pipeline) Init(ctx context.Context, projectRoot string) error {
	if err := p.ws.Prepare(ctx); err != nil {
		return fmt.Errorf("workspace prepare: %w", err)
	}
	if err := p.aud.Init(ctx, projectRoot); err != nil {
		return fmt.Errorf("auditor init: %w", err)
	}
	return nil
}

// Record saves a step manually. Called by the agent after completing work.
func (p *Pipeline) Record(ctx context.Context, projectRoot string, input auditor.StepInput) (string, error) {
	clean, err := p.ws.IsClean(ctx)
	if err != nil {
		return "", fmt.Errorf("workspace check: %w", err)
	}
	if !clean {
		return "", workspace.ErrDirtyWorkspace
	}
	return p.aud.Record(ctx, projectRoot, input)
}

// AnalyzeFailure classifies an error and suggests recovery.
func (p *Pipeline) AnalyzeFailure(ctx context.Context, failure recovery.RecoverableFailure) recovery.RecoveryPlan {
	return p.rec.Analyze(ctx, failure)
}

// ExecuteRewind rewinds files to a previous step.
func (p *Pipeline) ExecuteRewind(ctx context.Context, projectRoot string, plan recovery.RecoveryPlan) error {
	return p.rec.ExecuteRewind(ctx, plan, p.aud, projectRoot)
}
