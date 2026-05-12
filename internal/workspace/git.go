package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type gitWorkspace struct {
	root string
}

// New creates a Workspace backed by git at the given project root.
func New(root string) Workspace {
	return &gitWorkspace{root: root}
}

func (w *gitWorkspace) IsClean(ctx context.Context) (bool, error) {
	if !w.isGitRepo() {
		return false, ErrNotGitRepo
	}

	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = w.root
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(out) == 0, nil
}

func (w *gitWorkspace) Conflicts(ctx context.Context) ([]Conflict, error) {
	if !w.isGitRepo() {
		return nil, ErrNotGitRepo
	}

	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = w.root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var conflicts []Conflict
	for _, line := range lines {
		if line != "" {
			conflicts = append(conflicts, Conflict{
				FilePath: line,
				Reason:   "merge_conflict",
			})
		}
	}
	return conflicts, nil
}

func (w *gitWorkspace) Prepare(ctx context.Context) error {
	for _, dir := range []string{".gsd", ".regent"} {
		path := filepath.Join(w.root, dir)
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (w *gitWorkspace) CurrentHEAD(ctx context.Context) (string, error) {
	if !w.isGitRepo() {
		return "", ErrNotGitRepo
	}

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = w.root
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (w *gitWorkspace) isGitRepo() bool {
	info, err := os.Stat(filepath.Join(w.root, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir()
}
