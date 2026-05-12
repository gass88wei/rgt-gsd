package auditor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type rgtAuditor struct {
	rgtPath string
}

// New creates an Auditor backed by the rgt CLI.
// If rgtPath is empty, searches: same directory as running binary, then PATH.
func New(rgtPath string) Auditor {
	if rgtPath == "" {
		rgtPath = resolveRgt()
	}
	return &rgtAuditor{rgtPath: rgtPath}
}

func resolveRgt() string {
	name := "rgt"
	if runtime.GOOS == "windows" {
		name = "rgt.exe"
	}

	// Look next to current binary first
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// Fallback to PATH
	return name
}

func (a *rgtAuditor) Init(ctx context.Context, projectRoot string) error {
	cmd := exec.CommandContext(ctx, a.rgtPath, "init")
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		// rgt init is idempotent — already initialized is fine
		if strings.Contains(string(out), "already") {
			return nil
		}
		return fmt.Errorf("rgt init: %w: %s", err, string(out))
	}
	return nil
}

func (a *rgtAuditor) Record(ctx context.Context, projectRoot string, input StepInput) (string, error) {
	// rgt hook --session <id> --cause <json> --transcript <json>
	causeJSON, _ := json.Marshal(input.Cause)
	transcriptJSON, _ := json.Marshal(input.Transcript)

	cmd := exec.CommandContext(ctx, a.rgtPath,
		"hook",
		"--session", input.SessionID,
		"--cause", string(causeJSON),
		"--transcript", string(transcriptJSON),
	)
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("rgt record: %w: %s", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

func (a *rgtAuditor) Blame(ctx context.Context, projectRoot string, filePath string, line int) (BlameEntry, error) {
	cmd := exec.CommandContext(ctx, a.rgtPath, "blame", filePath, "-L", strconv.Itoa(line))
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return BlameEntry{}, fmt.Errorf("rgt blame: %w: %s", err, string(out))
	}

	// Parse rgt blame output: "hash session_id tool_name timestamp"
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) < 2 {
		return BlameEntry{}, fmt.Errorf("unexpected rgt blame output: %s", string(out))
	}

	entry := BlameEntry{StepHash: parts[0]}
	if len(parts) > 1 {
		entry.SessionID = parts[1]
	}
	if len(parts) > 2 {
		entry.Cause.ToolName = parts[2]
	}
	return entry, nil
}

func (a *rgtAuditor) Log(ctx context.Context, projectRoot string, sessionID string) ([]Step, error) {
	args := []string{"log", "--format", "json"}
	if sessionID != "" {
		args = append(args, "--session", sessionID)
	}

	cmd := exec.CommandContext(ctx, a.rgtPath, args...)
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("rgt log: %w: %s", err, string(out))
	}

	var steps []Step
	if err := json.Unmarshal(out, &steps); err != nil {
		// Fallback: parse line by line
		return nil, fmt.Errorf("rgt log parse: %w", err)
	}
	return steps, nil
}

func (a *rgtAuditor) Rewind(ctx context.Context, projectRoot string, hash string) error {
	// rgt rewind is not implemented yet in re_gent v0.1.2
	// For now, use git checkout of the tree snapshot
	cmd := exec.CommandContext(ctx, a.rgtPath, "show", hash)
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rgt rewind: %w: %s", err, string(out))
	}
	return nil
}

func (a *rgtAuditor) Diff(ctx context.Context, projectRoot string, hashA, hashB string) ([]FileDiff, error) {
	cmd := exec.CommandContext(ctx, a.rgtPath, "show", hashA, hashB, "--diff")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("rgt diff: %w: %s", err, string(out))
	}

	var diffs []FileDiff
	if err := json.Unmarshal(out, &diffs); err != nil {
		return nil, fmt.Errorf("rgt diff parse: %w", err)
	}
	return diffs, nil
}
