package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/gass88wei/rgt-gsd/internal/auditor"
	"github.com/gass88wei/rgt-gsd/internal/plan"
	"github.com/gass88wei/rgt-gsd/internal/recovery"
	"github.com/gass88wei/rgt-gsd/internal/workspace"
)

// MCP JSON-RPC types
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// MCP types
type initializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type toolDefinition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema inputSchema `json:"inputSchema"`
}

type inputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type toolsListResult struct {
	Tools []toolDefinition `json:"tools"`
}

// Server handles MCP requests.
type Server struct {
	projectDir string
	aud        auditor.Auditor
	pln        plan.Plan
	ws         workspace.Workspace
	rec        recovery.Recovery
	mu         sync.Mutex
	w          io.Writer
}

// ServeMCP starts the MCP stdio server. Blocks until stdin closes.
func ServeMCP(projectDir string) error {
	s := &Server{
		projectDir: projectDir,
		aud:        auditor.New(""),
		pln:        plan.New(),
		ws:         workspace.New(projectDir),
		rec:        recovery.New(),
	}

	reader := bufio.NewReader(os.Stdin)
	s.w = os.Stdout

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue // skip malformed
		}

		// Notification (no id) — skip response
		if req.ID == nil {
			s.handleNotification(line)
			continue
		}

		resp := s.handleRequest(&req)
		if resp != nil {
			data, _ := json.Marshal(resp)
			fmt.Fprintf(os.Stdout, "%s\n", data)
		}
	}
}

func (s *Server) handleNotification(raw json.RawMessage) {
	// "notifications/initialized" — no-op
}

func (s *Server) handleRequest(req *rpcRequest) *rpcResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req.ID, req.Params)
	case "tools/list":
		return s.handleToolsList(req.ID)
	case "tools/call":
		return s.handleToolCall(req.ID, req.Params)
	default:
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: fmt.Sprintf("unknown method: %s", req.Method)},
		}
	}
}

func (s *Server) handleInitialize(id interface{}, params json.RawMessage) *rpcResponse {
	return &rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"serverInfo": serverInfo{
				Name:    "rgt-gsd",
				Version: "1.0.0",
			},
			"capabilities": map[string]interface{}{
				"tools": map[string]bool{},
			},
		},
	}
}

var tools = []toolDefinition{
	{Name: "rgt_gsd_plan_show", Description: "Show all tasks and completion status from ROADMAP.md", InputSchema: inputSchema{Type: "object", Properties: map[string]property{}}},
	{Name: "rgt_gsd_plan_next", Description: "Show the next unfinished task", InputSchema: inputSchema{Type: "object", Properties: map[string]property{}}},
	{Name: "rgt_gsd_plan_start", Description: "Mark a task as work-in-progress", InputSchema: inputSchema{Type: "object", Properties: map[string]property{"task": {Type: "string", Description: "Task name to mark as in-progress"}}, Required: []string{"task"}}},
	{Name: "rgt_gsd_archive", Description: "Complete a task: record snapshot + check in ROADMAP + git commit", InputSchema: inputSchema{Type: "object", Properties: map[string]property{"task": {Type: "string", Description: "Task name to archive"}}, Required: []string{"task"}}},
	{Name: "rgt_gsd_step", Description: "Record a step without marking any task done", InputSchema: inputSchema{Type: "object", Properties: map[string]property{"description": {Type: "string", Description: "What this step did"}}, Required: []string{"description"}}},
	{Name: "rgt_gsd_inspect", Description: "Open a step archive to see what files changed", InputSchema: inputSchema{Type: "object", Properties: map[string]property{"hash": {Type: "string", Description: "Step hash prefix to inspect"}}, Required: []string{"hash"}}},
	{Name: "rgt_gsd_blame", Description: "Find which step last modified a file", InputSchema: inputSchema{Type: "object", Properties: map[string]property{"file": {Type: "string", Description: "File path relative to project root"}}, Required: []string{"file"}}},
	{Name: "rgt_gsd_log", Description: "View step history", InputSchema: inputSchema{Type: "object", Properties: map[string]property{"limit": {Type: "number", Description: "Max steps (default 20)"}, "grep": {Type: "string", Description: "Filter by task name containing text"}}}},
	{Name: "rgt_gsd_recover", Description: "Analyze an error and suggest recovery action", InputSchema: inputSchema{Type: "object", Properties: map[string]property{"error_type": {Type: "string", Description: "tool_schema|network|deterministic|provider|policy_block|worktree_invalid|unknown"}, "message": {Type: "string", Description: "Error message"}}, Required: []string{"error_type"}}},
	{Name: "rgt_gsd_rewind", Description: "Restore files to a previous step", InputSchema: inputSchema{Type: "object", Properties: map[string]property{"hash": {Type: "string", Description: "Step hash to rewind to"}}, Required: []string{"hash"}}},
	{Name: "rgt_gsd_health", Description: "Check workspace + rgt status", InputSchema: inputSchema{Type: "object", Properties: map[string]property{}}},
}

func (s *Server) handleToolsList(id interface{}) *rpcResponse {
	return &rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  toolsListResult{Tools: tools},
	}
}

func (s *Server) handleToolCall(id interface{}, params json.RawMessage) *rpcResponse {
	var p callToolParams
	if err := json.Unmarshal(params, &p); err != nil {
		return &rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32602, Message: "invalid params"}}
	}

	result := s.callTool(p.Name, p.Arguments)
	return &rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

func (s *Server) callTool(name string, args json.RawMessage) toolResult {
	ctx := context.Background()

	switch name {
	case "rgt_gsd_plan_show":
		status, err := s.pln.Show(s.projectDir)
		if err != nil {
			return errResult(err.Error())
		}
		var text string
		for _, m := range status.Milestones {
			text += fmt.Sprintf("## %s\n", m.Name)
			for _, sl := range m.Slices {
				done := 0
				for _, t := range sl.Tasks {
					if t.Completed {
						done++
					}
				}
				text += fmt.Sprintf("  [%d/%d] %s\n", done, len(sl.Tasks), sl.Name)
				for _, t := range sl.Tasks {
					mark := " "
					if t.Completed {
						mark = "x"
					}
					text += fmt.Sprintf("    - [%s] %s\n", mark, t.Name)
				}
			}
		}
		return okResult(text)

	case "rgt_gsd_plan_next":
		step, err := s.pln.Next(s.projectDir)
		if err != nil {
			return errResult(err.Error())
		}
		if step.Type == "done" {
			return okResult("All tasks completed.")
		}
		return okResult(fmt.Sprintf("[%s] %s", step.Type, step.Description))

	case "rgt_gsd_plan_start":
		task := getString(args, "task")
		s.startWIP(task)
		return okResult(fmt.Sprintf("Started: %s", task))

	case "rgt_gsd_archive":
		task := getString(args, "task")
		hash, err := s.aud.Record(ctx, s.projectDir, auditor.StepInput{
			Cause: auditor.Cause{ToolName: "archive", ArgsJSON: task},
		})
		if err != nil {
			return errResult(err.Error())
		}
		if err := s.pln.MarkDone(s.projectDir, task); err != nil {
			return errResult(err.Error())
		}
		s.clearWIP(task)
		shortHash := hash
		if len(shortHash) > 12 {
			shortHash = shortHash[:12]
		}
		return okResult(fmt.Sprintf("archived [%s] %s", shortHash, task))

	case "rgt_gsd_step":
		desc := getString(args, "description")
		hash, err := s.aud.Record(ctx, s.projectDir, auditor.StepInput{
			Cause: auditor.Cause{ToolName: "step", ArgsJSON: desc},
		})
		if err != nil {
			return errResult(err.Error())
		}
		shortHash := hash
		if len(shortHash) > 12 {
			shortHash = shortHash[:12]
		}
		return okResult(fmt.Sprintf("step %s recorded", shortHash))

	case "rgt_gsd_inspect":
		hash := getString(args, "hash")
		steps, _ := s.aud.Log(ctx, s.projectDir, "")
		var info string
		for _, step := range steps {
			if stringsPrefix(step.Hash, hash) {
				info = fmt.Sprintf("Step: %s\nTool: %s\nWhen: %s\n",
					step.Hash, step.Cause.ToolName, step.Timestamp.Format("2006-01-02 15:04:05"))
				break
			}
		}
		return okResult(info)

	case "rgt_gsd_blame":
		file := getString(args, "file")
		entry, err := s.aud.Blame(ctx, s.projectDir, file, 0)
		if err != nil {
			return errResult(err.Error())
		}
		shortHash := entry.StepHash
		if len(shortHash) > 12 {
			shortHash = shortHash[:12]
		}
		return okResult(fmt.Sprintf("%s: last modified by %s (%s)", file, shortHash, entry.Cause.ToolName))

	case "rgt_gsd_log":
		limit := getInt(args, "limit", 20)
		grep := getString(args, "grep")
		steps, err := s.aud.Log(ctx, s.projectDir, "")
		if err != nil {
			return errResult(err.Error())
		}
		var text string
		count := 0
		for _, step := range steps {
			if grep != "" && !stringsContains(step.Cause.ArgsJSON, grep) && !stringsContains(step.Cause.ToolName, grep) {
				continue
			}
			shortHash := step.Hash
			if len(shortHash) > 12 {
				shortHash = shortHash[:12]
			}
			text += fmt.Sprintf("%s  %s  %s\n", shortHash, step.Cause.ToolName, step.Timestamp.Format("15:04:05"))
			count++
			if count >= limit {
				break
			}
		}
		return okResult(text)

	case "rgt_gsd_recover":
		errorType := getString(args, "error_type")
		message := getString(args, "message")
		plan := s.rec.Analyze(ctx, recovery.RecoverableFailure{
			ErrorType:    errorType,
			ErrorMessage: message,
		})
		text := fmt.Sprintf("Action: %s\nReason: %s", plan.Action, plan.Reason)
		if plan.HumanNote != "" {
			text += "\nNote: " + plan.HumanNote
		}
		return okResult(text)

	case "rgt_gsd_rewind":
		hash := getString(args, "hash")
		if err := s.aud.Rewind(ctx, s.projectDir, hash); err != nil {
			return errResult(err.Error())
		}
		return okResult("Rewind complete.")

	case "rgt_gsd_health":
		clean, err := s.ws.IsClean(ctx)
		wsStatus := "OK (clean)"
		if err != nil {
			wsStatus = fmt.Sprintf("FAIL: %v", err)
		} else if !clean {
			wsStatus = "WARN (uncommitted)"
		}
		rgtStatus := "OK"
		if err := s.aud.Init(ctx, s.projectDir); err != nil {
			rgtStatus = fmt.Sprintf("FAIL: %v", err)
		}
		return okResult(fmt.Sprintf("workspace: %s\nrgt: %s", wsStatus, rgtStatus))

	default:
		return errResult(fmt.Sprintf("unknown tool: %s", name))
	}
}

// WIP state (in-memory for MCP server, file-backed for CLI)
type wipState struct {
	Task      string
	StartedAt string
}

var wip wipState

func (s *Server) startWIP(task string) {
	wip = wipState{Task: task, StartedAt: "now"}
}

func (s *Server) clearWIP(task string) {
	if wip.Task == task || stringsPrefix(wip.Task, task) {
		wip = wipState{}
	}
}

// helpers
func okResult(text string) toolResult {
	return toolResult{Content: []toolContent{{Type: "text", Text: text}}}
}

func errResult(msg string) toolResult {
	return toolResult{Content: []toolContent{{Type: "text", Text: msg}}, IsError: true}
}

func getString(args json.RawMessage, key string) string {
	var m map[string]interface{}
	if err := json.Unmarshal(args, &m); err != nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

func getInt(args json.RawMessage, key string, defaultVal int) int {
	var m map[string]interface{}
	if err := json.Unmarshal(args, &m); err != nil {
		return defaultVal
	}
	if n, ok := m[key].(float64); ok {
		return int(n)
	}
	return defaultVal
}

func stringsPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func stringsContains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
