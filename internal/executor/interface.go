package executor

import (
	"context"
	"time"
)

// PlanSpec describes what gsd-2 should execute.
type PlanSpec struct {
	MilestoneID string
	Command     string // initial command, default "/gsd auto"
}

// ExecEvent is emitted by the executor as gsd-2 progresses.
type ExecEvent struct {
	Type      string    // "unit_started", "unit_completed", "blocked", "error", "completed", "cancelled", "timeout"
	UnitID    string
	UnitType  string    // "plan_milestone", "plan_slice", "exec", "complete_slice", etc.
	Timestamp time.Time
	Error     error
	Cost      CostInfo
}

// CostInfo tracks accumulated cost for a session.
type CostInfo struct {
	TotalCost    float64
	InputTokens  int
	OutputTokens int
}

// SessionStatus is a snapshot of the current gsd-2 session state.
type SessionStatus struct {
	SessionID   string
	Status      string // "starting", "running", "blocked", "completed", "error", "cancelled"
	CurrentUnit string
	Cost        CostInfo
	Error       string
}

// Executor manages gsd-2 execution sessions.
type Executor interface {
	// Run starts a gsd-2 session and returns an event channel.
	Run(ctx context.Context, projectDir string, plan PlanSpec) (<-chan ExecEvent, error)

	// Status returns the current status of a session.
	Status(ctx context.Context, sessionID string) (SessionStatus, error)

	// Stop terminates a running session.
	Stop(ctx context.Context, sessionID string) error

	// Resume continues execution from a specific unit.
	Resume(ctx context.Context, sessionID string, fromUnit string) (<-chan ExecEvent, error)

	// HealthCheck verifies gsd-pi is available.
	HealthCheck(ctx context.Context) error
}
