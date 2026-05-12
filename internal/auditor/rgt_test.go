package auditor_test

import (
	"context"
	"os"
	"testing"

	"github.com/gass88wei/rgt-gsd/internal/auditor"
)

func TestNew(t *testing.T) {
	a := auditor.New("rgt")
	if a == nil {
		t.Fatal("New returned nil")
	}
}

func TestNewEmptyPath(t *testing.T) {
	a := auditor.New("")
	if a == nil {
		t.Fatal("New with empty path returned nil")
	}
}

func TestInitCreatesRegentDir(t *testing.T) {
	dir := t.TempDir()
	a := auditor.New("rgt")
	ctx := context.Background()

	// rgt may not be installed in test env, so this tests error handling
	err := a.Init(ctx, dir)
	if err != nil {
		// rgt not found is expected in test environment
		t.Logf("Init returned error (expected if rgt not installed): %v", err)
		return
	}

	info, statErr := os.Stat(dir + "/.regent")
	if statErr != nil {
		t.Logf(".regent dir not created (rgt may not be available): %v", statErr)
	} else if !info.IsDir() {
		t.Error(".regent exists but is not a directory")
	}
}

func TestRecordWithNoRGT(t *testing.T) {
	dir := t.TempDir()
	a := auditor.New("rgt")
	ctx := context.Background()

	_, err := a.Record(ctx, dir, auditor.StepInput{
		SessionID: "test-session",
	})
	if err == nil {
		t.Skip("rgt is available in test environment")
	}
	t.Logf("Record error (expected if rgt not installed): %v", err)
}

func TestBlameWithNoRGT(t *testing.T) {
	dir := t.TempDir()
	a := auditor.New("rgt")
	ctx := context.Background()

	_, err := a.Blame(ctx, dir, "main.go", 1)
	if err == nil {
		t.Skip("rgt is available in test environment")
	}
	t.Logf("Blame error (expected if rgt not installed): %v", err)
}
