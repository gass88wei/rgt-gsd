package workspace

import "context"

// Conflict represents a file that prevents clean execution.
type Conflict struct {
	FilePath string
	Reason   string
}

// Workspace checks git state and prepares the environment for execution.
type Workspace interface {
	// IsClean returns true if the working tree has no uncommitted changes.
	IsClean(ctx context.Context) (bool, error)

	// Conflicts returns files in conflict state (merge conflicts, etc.).
	Conflicts(ctx context.Context) ([]Conflict, error)

	// Prepare ensures .gsd/ and .regent/ directories exist.
	Prepare(ctx context.Context) error

	// CurrentHEAD returns the current git HEAD hash.
	CurrentHEAD(ctx context.Context) (string, error)
}
