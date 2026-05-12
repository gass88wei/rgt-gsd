package auditor

import (
	"context"
	"time"
)

// StepInput is the input for recording an agent step.
type StepInput struct {
	SessionID  string
	Cause      Cause
	Transcript []Message
	Effects    []Effect
}

// Cause describes what tool call produced this step.
type Cause struct {
	ToolUseID  string
	ToolName   string
	ArgsJSON   string
	ResultJSON string
}

// Message is a conversation message.
type Message struct {
	Role    string
	Content string
}

// Effect is a side effect that cannot be rewound.
type Effect struct {
	Type        string // "http_call", "db_write", etc.
	Description string
}

// Step is a recorded agent action.
type Step struct {
	Hash      string
	SessionID string
	Parent    string
	Cause     Cause
	Timestamp time.Time
}

// BlameEntry shows which step introduced a line.
type BlameEntry struct {
	StepHash  string
	SessionID string
	Cause     Cause
	Timestamp time.Time
}

// FileDiff represents changes between two steps.
type FileDiff struct {
	FilePath    string
	AddedLines  int
	RemovedLines int
}

// Auditor provides version control for AI agent activity.
type Auditor interface {
	// Init initializes the .regent/ directory.
	Init(ctx context.Context, projectRoot string) error

	// Record records a new step (workspace snapshot + cause + transcript).
	Record(ctx context.Context, projectRoot string, input StepInput) (hash string, err error)

	// Blame returns which step introduced a specific line.
	Blame(ctx context.Context, projectRoot string, filePath string, line int) (BlameEntry, error)

	// Log returns the step history for a session.
	Log(ctx context.Context, projectRoot string, sessionID string) ([]Step, error)

	// Rewind restores files to a specific step.
	Rewind(ctx context.Context, projectRoot string, hash string) error

	// Diff returns file changes between two steps.
	Diff(ctx context.Context, projectRoot string, hashA, hashB string) ([]FileDiff, error)
}
