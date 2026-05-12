package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/gass88wei/rgt-gsd/internal/auditor"
	"github.com/gass88wei/rgt-gsd/internal/pipeline"
	"github.com/gass88wei/rgt-gsd/internal/plan"
	"github.com/gass88wei/rgt-gsd/internal/recovery"
	"github.com/gass88wei/rgt-gsd/internal/workspace"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "rgt-gsd",
		Short: "rgt-gsd — AI agent audit trail and recovery toolbox",
		Long: `rgt-gsd combines re_gent (version control for AI agents) with plan tracking and recovery.

It does NOT call LLM APIs. It is a toolbox your agent uses to:
  - Record what it did (record)
  - Trace code to prompts (blame)
  - Rewind on failure (rewind)
  - Track project progress (plan)`,
	}

	rootCmd.AddCommand(initCmd())
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
		Use:   "blame <file>:<line>",
		Short: "Show which step introduced a specific line",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			aud := auditor.New("")
			var filePath string
			var line int
			if _, err := fmt.Sscanf(args[0], "%[^:]:%d", &filePath, &line); err != nil {
				return fmt.Errorf("expected format <file>:<line>, got %s", args[0])
			}
			entry, err := aud.Blame(ctx, projectDir, filePath, line)
			if err != nil {
				return err
			}
			fmt.Printf("%s:%d  %s  %s\n", filePath, line, entry.StepHash[:8], entry.Cause.ToolName)
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")
	return cmd
}

func auditLogCmd() *cobra.Command {
	var projectDir, sessionID string
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
			for _, step := range steps {
				fmt.Printf("%s  %s  %s\n", step.Hash[:8], step.Cause.ToolName, step.Timestamp.Format("15:04:05"))
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")
	cmd.Flags().StringVarP(&sessionID, "session", "s", "", "session ID filter")
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
				fmt.Scanln(&answer)
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
