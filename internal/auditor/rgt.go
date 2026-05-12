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
	"time"
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
	// Generate stable session ID if not provided.
	sessionID := input.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("rgt-gsd-%d", time.Now().Unix())
	}

	payload := map[string]interface{}{
		"session_id":    sessionID,
		"tool_name":     input.Cause.ToolName,
		"tool_use_id":   input.Cause.ToolUseID,
		"tool_input":    input.Cause.ArgsJSON,
		"tool_response": input.Cause.ResultJSON,
		"cwd":           projectRoot,
	}
	payloadJSON, _ := json.Marshal(payload)

	cmd := exec.CommandContext(ctx, a.rgtPath, "hook")
	cmd.Dir = projectRoot
	cmd.Stdin = strings.NewReader(string(payloadJSON))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("rgt record: %w: %s", err, string(out))
	}

	// rgt hook succeeds silently. Get the latest step hash for this session.
	steps, logErr := a.Log(ctx, projectRoot, sessionID)
	if logErr != nil || len(steps) == 0 {
		return "", fmt.Errorf("rgt record: hook succeeded but log returned no step: %w", logErr)
	}
	return steps[0].Hash, nil
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
	// If a specific session is requested, query it directly
	if sessionID != "" {
		return a.logSession(ctx, projectRoot, sessionID)
	}

	// Otherwise enumerate all sessions, merge results
	ids, err := a.listSessions(ctx, projectRoot)
	if err != nil {
		return nil, err
	}

	var all []Step
	for _, id := range ids {
		steps, err := a.logSession(ctx, projectRoot, id)
		if err != nil {
			continue // skip broken sessions
		}
		all = append(all, steps...)
	}
	return all, nil
}

// listSessions returns all session IDs from rgt sessions --json.
func (a *rgtAuditor) listSessions(ctx context.Context, projectRoot string) ([]string, error) {
	cmd := exec.CommandContext(ctx, a.rgtPath, "sessions")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, nil
	}

	var ids []string
	for _, line := range strings.Split(string(out), "\n") {
		// Strip ANSI escape codes
		line = stripAnsi(line)
		line = strings.TrimSpace(line)
		// Line format: "Session: <id>" (after stripping ANSI prefix junk)
		if idx := strings.Index(line, "ession:"); idx >= 0 {
			rest := line[idx+7:] // after "ession:" (handles "S" stripped by ANSI)
			id := strings.TrimSpace(rest)
			if id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids, nil
}

// stripAnsi removes ANSI escape sequences from a string.
func stripAnsi(s string) string {
	for {
		start := strings.Index(s, "\x1b[")
		if start < 0 {
			break
		}
		end := start + 2
		for end < len(s) && s[end] != 'm' {
			end++
		}
		if end < len(s) {
			s = s[:start] + s[end+1:]
		} else {
			s = s[:start]
		}
	}
	return s
}

func (a *rgtAuditor) logSession(ctx context.Context, projectRoot, sessionID string) ([]Step, error) {
	cmd := exec.CommandContext(ctx, a.rgtPath, "log", "--json", "--session", sessionID)
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("rgt log: %w: %s", err, string(out))
	}

	outStr := strings.TrimSpace(string(out))
	if len(out) == 0 || outStr == "No sessions found." {
		return nil, nil
	}
	if !strings.HasPrefix(outStr, "{") {
		return nil, nil
	}

	var wrapper struct {
		Steps []struct {
			Hash      string `json:"hash"`
			Timestamp string `json:"timestamp"`
			Tool      string `json:"tool"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(out, &wrapper); err != nil {
		return nil, fmt.Errorf("rgt log parse: %w", err)
	}
	steps := make([]Step, 0, len(wrapper.Steps))
	for _, s := range wrapper.Steps {
		ts, _ := time.Parse(time.RFC3339, s.Timestamp)
		steps = append(steps, Step{
			Hash:      s.Hash,
			Cause:     Cause{ToolName: s.Tool},
			Timestamp: ts,
		})
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
