package executor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

type gsdExecutor struct {
	gsdPath  string
	sessions map[string]*sessionState
	mu       sync.Mutex
}

type sessionState struct {
	id     string
	cmd    *exec.Cmd
	events chan ExecEvent
	cancel context.CancelFunc
}

// New creates an Executor backed by the gsd-pi CLI.
func New(gsdPath string) Executor {
	if gsdPath == "" {
		gsdPath = "gsd-pi"
	}
	return &gsdExecutor{
		gsdPath:  gsdPath,
		sessions: make(map[string]*sessionState),
	}
}

func (e *gsdExecutor) HealthCheck(ctx context.Context) error {
	_, err := exec.LookPath(e.gsdPath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrGSDNotFound, err)
	}
	return nil
}

func (e *gsdExecutor) Run(ctx context.Context, projectDir string, plan PlanSpec) (<-chan ExecEvent, error) {
	if err := e.HealthCheck(ctx); err != nil {
		return nil, err
	}

	e.mu.Lock()
	if _, ok := e.sessions[projectDir]; ok {
		e.mu.Unlock()
		return nil, ErrAlreadyActive
	}
	e.mu.Unlock()

	events := make(chan ExecEvent, 32)
	sessionID := fmt.Sprintf("gsd-%d", time.Now().UnixNano())

	ctx, cancel := context.WithCancel(ctx)

	command := plan.Command
	if command == "" {
		command = "/gsd auto"
	}

	// gsd-pi --mode rpc is for MCP server mode.
	// For simple CLI execution, use gsd-pi directly with the command.
	cmd := exec.CommandContext(ctx, e.gsdPath, command)
	cmd.Dir = projectDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("%w: %v", ErrSessionFailed, err)
	}

	state := &sessionState{
		id:     sessionID,
		cmd:    cmd,
		events: events,
		cancel: cancel,
	}

	e.mu.Lock()
	e.sessions[projectDir] = state
	e.sessions[sessionID] = state
	e.mu.Unlock()

	go e.monitorProcess(ctx, state, stdout, stderr)

	return events, nil
}

func (e *gsdExecutor) monitorProcess(ctx context.Context, state *sessionState, stdout, stderr io.ReadCloser) {
	defer close(state.events)

	// Fan-in stdout and stderr
	done := make(chan struct{}, 2)
	go func() {
		e.parseOutput(state, stdout, "stdout")
		done <- struct{}{}
	}()
	go func() {
		e.parseOutput(state, stderr, "stderr")
		done <- struct{}{}
	}()

	// Wait for process to finish
	err := state.cmd.Wait()

	// Wait for output readers
	<-done
	<-done

	if err != nil {
		if ctx.Err() != nil {
			state.events <- ExecEvent{
				Type:      "cancelled",
				Timestamp: time.Now(),
			}
		} else {
			state.events <- ExecEvent{
				Type:      "error",
				Timestamp: time.Now(),
				Error:     fmt.Errorf("gsd-2 process: %w", err),
			}
		}
	} else {
		state.events <- ExecEvent{
			Type:      "completed",
			Timestamp: time.Now(),
		}
	}
}

func (e *gsdExecutor) parseOutput(state *sessionState, r io.Reader, source string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()

		// Try to parse as JSON event first
		var event ExecEvent
		if err := json.Unmarshal([]byte(line), &event); err == nil {
			event.Timestamp = time.Now()
			select {
			case state.events <- event:
			default:
				// channel full, drop event
			}
			continue
		}

		// Heuristic: detect gsd-2 unit completion from text output
		// gsd-2 typically outputs "Unit completed: <name>" or similar patterns
		event = e.parseTextEvent(line)
		if event.Type != "" {
			event.Timestamp = time.Now()
			select {
			case state.events <- event:
			default:
			}
		}
	}
}

func (e *gsdExecutor) parseTextEvent(line string) ExecEvent {
	// Common gsd-2 output patterns
	switch {
	case containsAny(line, "auto-mode stopped", "step-mode stopped"):
		return ExecEvent{Type: "completed"}
	case containsAny(line, "blocked:", "BLOCKED"):
		return ExecEvent{Type: "blocked"}
	case containsAny(line, "error:", "ERROR", "failed:", "FAILED"):
		return ExecEvent{Type: "error", Error: fmt.Errorf("%s", line)}
	default:
		return ExecEvent{}
	}
}

func (e *gsdExecutor) Status(ctx context.Context, sessionID string) (SessionStatus, error) {
	e.mu.Lock()
	state, ok := e.sessions[sessionID]
	e.mu.Unlock()
	if !ok {
		return SessionStatus{}, ErrSessionNotFound
	}

	return SessionStatus{
		SessionID: state.id,
		Status:    "running",
	}, nil
}

func (e *gsdExecutor) Stop(ctx context.Context, sessionID string) error {
	e.mu.Lock()
	state, ok := e.sessions[sessionID]
	e.mu.Unlock()
	if !ok {
		return ErrSessionNotFound
	}

	state.cancel()
	return nil
}

func (e *gsdExecutor) Resume(ctx context.Context, sessionID string, fromUnit string) (<-chan ExecEvent, error) {
	e.mu.Lock()
	state, ok := e.sessions[sessionID]
	e.mu.Unlock()
	if !ok {
		return nil, ErrSessionNotFound
	}

	// For resume, we run gsd-pi with a resume command
	events := make(chan ExecEvent, 32)
	ctx, cancel := context.WithCancel(ctx)

	cmd := exec.CommandContext(ctx, e.gsdPath, "/gsd resume", "--from", fromUnit)
	cmd.Dir = state.cmd.Dir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("%w: %v", ErrSessionFailed, err)
	}

	newState := &sessionState{
		id:     sessionID,
		cmd:    cmd,
		events: events,
		cancel: cancel,
	}
	e.mu.Lock()
	e.sessions[sessionID] = newState
	e.mu.Unlock()

	go e.monitorProcess(ctx, newState, stdout, stderr)

	return events, nil
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) {
			// Simple case-insensitive prefix check
			for i := 0; i <= len(s)-len(sub); i++ {
				if equalFold(s[i:i+len(sub)], sub) {
					return true
				}
			}
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
