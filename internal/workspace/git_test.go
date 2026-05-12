package workspace_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gass88wei/rgt-gsd/internal/workspace"
)

func TestNew(t *testing.T) {
	ws := workspace.New("/tmp")
	if ws == nil {
		t.Fatal("New returned nil")
	}
}

func TestIsNotGitRepo(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir)
	ctx := context.Background()

	clean, err := ws.IsClean(ctx)
	if err != workspace.ErrNotGitRepo {
		t.Fatalf("expected ErrNotGitRepo, got %v", err)
	}
	if clean {
		t.Fatal("expected false for non-git dir")
	}
}

func TestConflictsNotGitRepo(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir)
	ctx := context.Background()

	_, err := ws.Conflicts(ctx)
	if err != workspace.ErrNotGitRepo {
		t.Fatalf("expected ErrNotGitRepo, got %v", err)
	}
}

func TestCurrentHEADNotGitRepo(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir)
	ctx := context.Background()

	_, err := ws.CurrentHEAD(ctx)
	if err != workspace.ErrNotGitRepo {
		t.Fatalf("expected ErrNotGitRepo, got %v", err)
	}
}

func TestPrepareCreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir)
	ctx := context.Background()

	err := ws.Prepare(ctx)
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	for _, sub := range []string{".gsd", ".regent"} {
		info, err := os.Stat(filepath.Join(dir, sub))
		if err != nil {
			t.Errorf("%s not created: %v", sub, err)
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", sub)
		}
	}
}

func TestPrepareIdempotent(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.New(dir)
	ctx := context.Background()

	if err := ws.Prepare(ctx); err != nil {
		t.Fatalf("first Prepare failed: %v", err)
	}
	if err := ws.Prepare(ctx); err != nil {
		t.Fatalf("second Prepare failed: %v", err)
	}
}

func TestPrepareReadOnlyParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission model differs on Windows")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod failed: %v", err)
	}
	defer os.Chmod(dir, 0o700)

	ws := workspace.New(filepath.Join(dir, "subdir"))
	ctx := context.Background()

	err := ws.Prepare(ctx)
	if err == nil {
		t.Fatal("expected permission error, got nil")
	}
}
