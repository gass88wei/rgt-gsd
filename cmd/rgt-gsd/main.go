package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gass88wei/rgt-gsd/internal/auditor"
	"github.com/gass88wei/rgt-gsd/internal/pipeline"
	"github.com/gass88wei/rgt-gsd/internal/plan"
	"github.com/gass88wei/rgt-gsd/internal/recovery"
	"github.com/gass88wei/rgt-gsd/internal/server"
	"github.com/gass88wei/rgt-gsd/internal/state"
	"github.com/gass88wei/rgt-gsd/internal/workspace"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runCmdOut(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "rgt-gsd",
		Short: "rgt-gsd — AI agent audit trail and recovery toolbox",
		Long: `rgt-gsd combines re_gent (version control for AI agents) with plan tracking and recovery.

It does NOT call LLM APIs. It is a toolbox your agent uses to:
  - Archive completed tasks (archive)
  - Inspect step archives (inspect)
  - Trace code to prompts (blame)
  - Track project progress (plan)
  - Serve MCP tools (serve)`,
	}

	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(serveCmd())
	rootCmd.AddCommand(archiveCmd())
	rootCmd.AddCommand(stepCmd())
	rootCmd.AddCommand(inspectCmd())
	rootCmd.AddCommand(recordCmd())
	rootCmd.AddCommand(auditCmd())
	rootCmd.AddCommand(planCmd())
	rootCmd.AddCommand(recoverCmd())
	rootCmd.AddCommand(healthCmd())
	rootCmd.AddCommand(versionCmd())

	cobra.EnableCommandSorting = false

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("rgt-gsd %s\n", version)
			fmt.Printf("  commit: %s\n", commit)
			fmt.Printf("  date:   %s\n", date)
		},
	}
}

// --- init ---

func initCmd() *cobra.Command {
	var projectDir string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize rgt-gsd in a project (.gsd/ + .regent/)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			p := pipeline.New(
				workspace.New(projectDir),
				auditor.New(""),
				recovery.New(),
			)
			if err := p.Init(ctx, projectDir); err != nil {
				return err
			}
			fmt.Printf("rgt-gsd initialized in %s\n", projectDir)
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")
	return cmd
}

// --- serve ---

func serveCmd() *cobra.Command {
	var projectDir string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start MCP server over stdio (for agent framework integration)",
		Long:  "Starts an MCP server reading JSON-RPC from stdin, writing to stdout. Configure your agent framework's MCP settings to call rgt-gsd serve.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return server.ServeMCP(projectDir)
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")
	return cmd
}

// --- record ---

func recordCmd() *cobra.Command {
	var projectDir string
	cmd := &cobra.Command{
		Use:   "record <description>",
		Short: "Record a step manually (call after your agent completes work)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			p := pipeline.New(
				workspace.New(projectDir),
				auditor.New(""),
				recovery.New(),
			)
			hash, err := p.Record(ctx, projectDir, auditor.StepInput{
				Cause: auditor.Cause{
					ToolName: "manual/record",
					ArgsJSON: args[0],
				},
			})
			if err != nil {
				return err
			}
			if len(hash) >= 12 {
				fmt.Printf("step %s recorded\n", hash[:12])
			} else if hash != "" {
				fmt.Printf("step %s recorded\n", hash)
			} else {
				fmt.Println("step recorded")
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")
	return cmd
}

// --- audit ---

func auditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Version control commands (blame, log, diff)",
	}
	cmd.AddCommand(auditBlameCmd())
	cmd.AddCommand(auditLogCmd())
	return cmd
}

func auditBlameCmd() *cobra.Command {
	var projectDir string
	cmd := &cobra.Command{
		Use:   "blame <file>",
		Short: "Show which step last modified a file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			aud := auditor.New("")
			entry, err := aud.Blame(ctx, projectDir, args[0], 0)
			if err != nil {
				return err
			}
			shortHash := entry.StepHash
			if len(shortHash) > 12 {
				shortHash = shortHash[:12]
			}
			fmt.Printf("%s: last modified by %s (%s)\n", args[0], shortHash, entry.Cause.ToolName)
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")
	return cmd
}

func auditLogCmd() *cobra.Command {
	var projectDir, sessionID, grep string
	var detail bool
	var limit int
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show step history",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			aud := auditor.New("")
			steps, err := aud.Log(ctx, projectDir, sessionID)
			if err != nil {
				return err
			}
			count := 0
			for _, step := range steps {
				if grep != "" && !strings.Contains(step.Cause.ArgsJSON, grep) && !strings.Contains(step.Cause.ToolName, grep) {
					continue
				}
				fmt.Printf("%s  %s  %s\n", step.Hash[:8], step.Cause.ToolName, step.Timestamp.Format("15:04:05"))
				if detail {
					showOut, _ := runCmdOut(ctx, projectDir, "rgt", "show", step.Hash)
					for _, line := range strings.Split(showOut, "\n") {
						line = strings.TrimSpace(line)
						if line != "" {
							fmt.Printf("  %s\n", line)
						}
					}
					fmt.Println()
				}
				count++
				if limit > 0 && count >= limit {
					break
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")
	cmd.Flags().StringVarP(&sessionID, "session", "s", "", "session ID filter")
	cmd.Flags().BoolVar(&detail, "detail", false, "show full step details")
	cmd.Flags().StringVar(&grep, "grep", "", "filter by task name")
	cmd.Flags().IntVarP(&limit, "limit", "n", 0, "max steps to show")
	return cmd
}

// --- plan ---

func planCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Project plan tracking (reads ROADMAP.md)",
	}
	cmd.AddCommand(planShowCmd())
	cmd.AddCommand(planNextCmd())
	cmd.AddCommand(planStartCmd())
	cmd.AddCommand(planStatusCmd())
	return cmd
}

func planShowCmd() *cobra.Command {
	var projectDir string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show full project status from ROADMAP.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := plan.New()
			status, err := p.Show(projectDir)
			if err != nil {
				return err
			}
			for _, m := range status.Milestones {
				fmt.Printf("## %s\n", m.Name)
				for _, s := range m.Slices {
					done := 0
					for _, t := range s.Tasks {
						if t.Completed {
							done++
						}
					}
					fmt.Printf("  [%d/%d] %s\n", done, len(s.Tasks), s.Name)
					for _, t := range s.Tasks {
						mark := " "
						if t.Completed {
							mark = "x"
						}
						fmt.Printf("    - [%s] %s\n", mark, t.Name)
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")
	return cmd
}

func planNextCmd() *cobra.Command {
	var projectDir string
	cmd := &cobra.Command{
		Use:   "next",
		Short: "Show the next unfinished task",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := plan.New()
			step, err := p.Next(projectDir)
			if err != nil {
				return err
			}
			if step.Type == "done" {
				fmt.Println("All tasks completed.")
				return nil
			}
			fmt.Printf("[%s] %s\n", step.Type, step.Description)
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")
	return cmd
}

func planStartCmd() *cobra.Command {
	var projectDir string
	cmd := &cobra.Command{
		Use:   "start <task>",
		Short: "Mark a task as work-in-progress",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return saveWIP(projectDir, args[0])
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")
	return cmd
}

func planStatusCmd() *cobra.Command {
	var projectDir string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current WIP (work-in-progress)",
		RunE: func(cmd *cobra.Command, args []string) error {
			drifts, _ := state.Reconcile(projectDir)
			for _, d := range drifts {
				if d.Fixed {
					fmt.Printf("reconciled: %s\n", d.Detail)
				}
			}
			w, err := loadWIP(projectDir)
			if err != nil || w.Task == "" {
				fmt.Println("No task in progress.")
				return nil
			}
			fmt.Printf("WIP: %s (started %s)\n", w.Task, w.StartedAt)
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")
	return cmd
}

// WIP state helpers
type wipData struct {
	Task      string `json:"task"`
	StartedAt string `json:"started_at"`
}

func saveWIP(dir, task string) error {
	w := wipData{Task: task, StartedAt: time.Now().Format("15:04:05")}
	data, _ := json.Marshal(w)
	_ = os.MkdirAll(filepath.Join(dir, ".rgt-gsd"), 0755)
	return os.WriteFile(filepath.Join(dir, ".rgt-gsd", "wip.json"), data, 0644)
}

func loadWIP(dir string) (wipData, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".rgt-gsd", "wip.json"))
	if err != nil {
		return wipData{}, err
	}
	var w wipData
	return w, json.Unmarshal(data, &w)
}

// --- recover ---

func recoverCmd() *cobra.Command {
	var projectDir, errorType, errorMsg, failedStep string
	cmd := &cobra.Command{
		Use:   "recover",
		Short: "Analyze a failure and suggest recovery plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			p := pipeline.New(
				workspace.New(projectDir),
				auditor.New(""),
				recovery.New(),
			)
			plan := p.AnalyzeFailure(ctx, recovery.RecoverableFailure{
				ErrorType:    errorType,
				ErrorMessage: errorMsg,
				FailedStep:   failedStep,
			})
			fmt.Printf("Action:    %s\n", plan.Action)
			fmt.Printf("Reason:    %s\n", plan.Reason)
			if plan.RewindTo != "" {
				fmt.Printf("Rewind to: %s\n", plan.RewindTo)
			}
			if plan.HumanNote != "" {
				fmt.Printf("Note:      %s\n", plan.HumanNote)
			}

			if plan.Action == "rewind" && failedStep != "" {
				fmt.Print("\nExecute rewind? [y/N]: ")
				var answer string
				_, _ = fmt.Scanln(&answer)
				if answer == "y" || answer == "Y" {
					if err := p.ExecuteRewind(ctx, projectDir, plan); err != nil {
						return err
					}
					fmt.Println("Rewind complete.")
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")
	cmd.Flags().StringVar(&errorType, "type", "unknown", "error type (tool_schema/network/deterministic/...)")
	cmd.Flags().StringVar(&errorMsg, "msg", "", "error message")
	cmd.Flags().StringVar(&failedStep, "step", "", "failed step hash")
	return cmd
}

// --- health ---

func healthCmd() *cobra.Command {
	var projectDir string
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check workspace, rgt, and gsd-pi availability",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			ws := workspace.New(projectDir)

			clean, err := ws.IsClean(ctx)
			if err != nil {
				fmt.Printf("workspace: FAIL — %v\n", err)
			} else if clean {
				fmt.Println("workspace: OK (clean)")
			} else {
				fmt.Println("workspace: WARN (uncommitted changes)")
			}

			aud := auditor.New("")
			if err := aud.Init(ctx, projectDir); err != nil {
				fmt.Printf("rgt:       FAIL — %v\n", err)
			} else {
				fmt.Println("rgt:       OK")
			}

			return nil
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")
	return cmd
}

// --- step ---

func stepCmd() *cobra.Command {
	var projectDir, task string
	var done bool
	cmd := &cobra.Command{
		Use:   "step <description>",
		Short: "Record a step (optionally associate with a task, optionally complete it)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			desc := args[0]
			p := plan.New()
			aud := auditor.New("")
			toolName := "step"
			if done {
				toolName = "archive"
			}
			hash, err := aud.Record(ctx, projectDir, auditor.StepInput{
				Cause: auditor.Cause{ToolName: toolName, ArgsJSON: desc},
			})
			if err != nil {
				return fmt.Errorf("record: %w", err)
			}
			shortHash := hash
			if len(shortHash) > 12 {
				shortHash = shortHash[:12]
			}
			if task != "" && done {
				if err := p.MarkDone(projectDir, task); err != nil {
					return fmt.Errorf("mark done: %w", err)
				}
				commitMsg := fmt.Sprintf("archive: %s [%s]", task, shortHash)
				_ = runGit(projectDir, "add", "-A")
				_ = runGit(projectDir, "commit", "-m", commitMsg)
				fmt.Printf("archived [%s] %s\n", shortHash, task)
			} else if task != "" {
				commitMsg := fmt.Sprintf("step: %s [%s]", desc, shortHash)
				_ = runGit(projectDir, "add", "-A")
				_ = runGit(projectDir, "commit", "-m", commitMsg)
				fmt.Printf("step %s [%s]\n", shortHash, desc)
			} else if done {
				fmt.Printf("archived [%s] %s\n", shortHash, desc)
			} else {
				fmt.Printf("step %s recorded\n", shortHash)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")
	cmd.Flags().StringVarP(&task, "task", "t", "", "associate with this task")
	cmd.Flags().BoolVar(&done, "done", false, "mark task as complete")
	return cmd
}

// --- archive ---

func archiveCmd() *cobra.Command {
	var projectDir string
	cmd := &cobra.Command{
		Use:   "archive <task-name>",
		Short: "Record step + mark task done in ROADMAP.md + git commit",
		Long: `Completes a task: records a re_gent step, checks the task in ROADMAP.md,
and creates a git commit linking the step hash.

This keeps completed tasks out of the agent's context.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			taskName := args[0]
			p := plan.New()
			aud := auditor.New("")

			diffOut, _ := runCmdOut(ctx, projectDir, "git", "diff", "--stat", "HEAD~1", "HEAD")
			hash, err := aud.Record(ctx, projectDir, auditor.StepInput{
				Cause: auditor.Cause{
					ToolName:   "archive",
					ArgsJSON:   taskName,
					ResultJSON: diffOut,
				},
			})
			if err != nil {
				return fmt.Errorf("record: %w", err)
			}
			shortHash := hash
			if len(shortHash) >= 12 {
				shortHash = shortHash[:12]
			}

			if err := p.MarkDone(projectDir, taskName); err != nil {
				return fmt.Errorf("mark done: %w", err)
			}

			commitMsg := fmt.Sprintf("archive: %s [%s]", taskName, shortHash)
			if err := runGit(projectDir, "add", "-A"); err != nil {
				fmt.Fprintf(os.Stderr, "warning: git add: %v\n", err)
			}
			if err := runGit(projectDir, "commit", "-m", commitMsg); err != nil {
				fmt.Fprintf(os.Stderr, "warning: git: %v\n", err)
			}

			fmt.Printf("archived [%s] %s\n", shortHash, taskName)
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")
	return cmd
}

// --- inspect ---

func inspectCmd() *cobra.Command {
	var projectDir string
	cmd := &cobra.Command{
		Use:   "inspect <hash>",
		Short: "Open a step archive to see what changed and why",
		Long: `Shows the full details of a recorded step: what files changed,
what prompt caused it, and the diff. Use when debugging a failure.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			hash := args[0]
			aud := auditor.New("")

			// Try log first for metadata
			steps, _ := aud.Log(ctx, projectDir, "")
			for _, s := range steps {
				if strings.HasPrefix(s.Hash, hash) {
					fmt.Printf("Step:     %s\n", s.Hash)
					fmt.Printf("Tool:     %s\n", s.Cause.ToolName)
					fmt.Printf("When:     %s\n", s.Timestamp.Format("2006-01-02 15:04:05"))
					fmt.Println("---")
					break
				}
			}

			// rgt show — step details from re_gent
			showOut, err := runCmdOut(ctx, projectDir, "rgt", "show", hash)
			if err != nil {
				return fmt.Errorf("step %s not found: %w", hash, err)
			}
			fmt.Println(showOut)

			// Git diff: filter out .regent/ and binary noise
			prefix := hash
			if len(prefix) > 12 {
				prefix = prefix[:12]
			}
			gitLog, _ := runCmdOut(ctx, projectDir, "git", "log", "--oneline", "--grep", prefix, "-1")
			if gitLog != "" {
				fmt.Println()
				fmt.Println("--- files changed ---")
				diff, _ := runCmdOut(ctx, projectDir, "git", "diff", "--stat", "HEAD~1", "HEAD", "--", ".", ":!.regent/", ":!.rgt-gsd/", ":!*.exe", ":!*.json", ":!go.sum")
				if diff != "" {
					fmt.Println(diff)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")
	return cmd
}
