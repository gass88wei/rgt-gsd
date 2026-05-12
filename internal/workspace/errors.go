package workspace

import "errors"

var (
	ErrNotGitRepo     = errors.New("not a git repository")
	ErrDirtyWorkspace = errors.New("working tree has uncommitted changes")
	ErrConflicts      = errors.New("working tree has unresolved conflicts")
)
