package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/gass88wei/rgt-gsd/internal/auditor"
	"github.com/gass88wei/rgt-gsd/internal/executor"
	"github.com/gass88wei/rgt-gsd/internal/pipeline"
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
		Short: "rgt-gsd — AI agent orchestration with version control",
		Long: `rgt-gsd combines re_gent (version control for AI agents) and gsd-2 (autonomous execution engine).

Run 'rgt-gsd run' to execute a gsd-2 plan with automatic audit trail and recovery.
Run 'rgt-gsd audit blame' to trace which prompt wrote a specific line of code.`,
	}

	rootCmd.AddCommand(runCmd())
	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(auditCmd())
	rootCmd.AddCommand(execCmd())
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

func runCmd() *cobra.Command {
	var (
		planFile   string
		projectDir string
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute gsd-2 plan with audit trail and auto-recovery",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			ws := workspace.New(projectDir)
			aud := auditor.New("rgt")
			exec := executor.New("gsd-pi")
			rec := recovery.New()

			p := pipeline.New(ws, aud, exec, rec)

			var command string
			if planFile != "" {
				command = fmt.Sprintf("/gsd auto --plan %s", planFile)
			} else {
				command = "/gsd auto"
			}

			result, err := p.Run(ctx, projectDir, executor.PlanSpec{
				Command: command,
			})
			if err != nil {
				return err
			}

			fmt.Printf("Session: %s\n", result.SessionID)
			fmt.Printf("Steps recorded: %d\n", len(result.Steps))
			fmt.Printf("Total cost: $%.4f\n", result.Cost.TotalCost)
			fmt.Printf("Input tokens: %d\n", result.Cost.InputTokens)

			return nil
		},
	}

	cmd.Flags().StringVarP(&planFile, "plan", "p", "", "gsd-2 plan file path")
	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")

	return cmd
}

func initCmd() *cobra.Command {
	var projectDir string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize rgt-gsd in a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			ws := workspace.New(projectDir)
			aud := auditor.New("rgt")

			if err := ws.Prepare(ctx); err != nil {
				return fmt.Errorf("workspace prepare: %w", err)
			}
			if err := aud.Init(ctx, projectDir); err != nil {
				return fmt.Errorf("auditor init: %w", err)
			}

			fmt.Printf("rgt-gsd initialized in %s\n", projectDir)
			return nil
		},
	}

	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")

	return cmd
}

func auditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Version control commands (blame, log, rewind, diff)",
	}

	cmd.AddCommand(auditBlameCmd())
	cmd.AddCommand(auditLogCmd())

	return cmd
}

func auditBlameCmd() *cobra.Command {
	var projectDir string

	cmd := &cobra.Command{
		Use:   "blame <file>:<line>",
		Short: "Show which prompt wrote a specific line",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			aud := auditor.New("rgt")

			var filePath string
			var line int
			if _, err := fmt.Sscanf(args[0], "%[^:]:%d", &filePath, &line); err != nil {
				return fmt.Errorf("expected format <file>:<line>, got %s", args[0])
			}

			entry, err := aud.Blame(ctx, projectDir, filePath, line)
			if err != nil {
				return err
			}

			fmt.Printf("%s:%d  %s  %s  %s\n",
				filePath, line, entry.StepHash[:8], entry.SessionID, entry.Cause.ToolName)
			return nil
		},
	}

	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")

	return cmd
}

func auditLogCmd() *cobra.Command {
	var (
		projectDir string
		sessionID  string
	)

	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show step history for a session",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			aud := auditor.New("rgt")

			steps, err := aud.Log(ctx, projectDir, sessionID)
			if err != nil {
				return err
			}

			for _, step := range steps {
				fmt.Printf("%s  %s  %s\n",
					step.Hash[:8], step.Cause.ToolName, step.Timestamp.Format("15:04:05"))
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")
	cmd.Flags().StringVarP(&sessionID, "session", "s", "", "session ID filter")

	return cmd
}

func execCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec",
		Short: "Execution commands (status, stop)",
	}

	cmd.AddCommand(execStatusCmd())
	cmd.AddCommand(execStopCmd())

	return cmd
}

func execStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <session-id>",
		Short: "Show session status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			exec := executor.New("gsd-pi")

			status, err := exec.Status(ctx, args[0])
			if err != nil {
				return err
			}

			fmt.Printf("Session: %s\n", status.SessionID)
			fmt.Printf("Status:  %s\n", status.Status)
			fmt.Printf("Unit:    %s\n", status.CurrentUnit)
			fmt.Printf("Cost:    $%.4f\n", status.Cost.TotalCost)
			if status.Error != "" {
				fmt.Printf("Error:   %s\n", status.Error)
			}
			return nil
		},
	}

	return cmd
}

func execStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <session-id>",
		Short: "Stop a running session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			exec := executor.New("gsd-pi")

			if err := exec.Stop(ctx, args[0]); err != nil {
				return err
			}

			fmt.Printf("Session %s stopped\n", args[0])
			return nil
		},
	}

	return cmd
}

func healthCmd() *cobra.Command {
	var projectDir string

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check workspace and tool availability",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			ws := workspace.New(projectDir)
			exec := executor.New("gsd-pi")

			clean, err := ws.IsClean(ctx)
			if err != nil {
				fmt.Printf("workspace: FAIL — %v\n", err)
			} else if clean {
				fmt.Println("workspace: OK (clean)")
			} else {
				fmt.Println("workspace: WARN (uncommitted changes)")
			}

			if err := exec.HealthCheck(ctx); err != nil {
				fmt.Printf("gsd-pi:    FAIL — %v\n", err)
			} else {
				fmt.Println("gsd-pi:    OK")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&projectDir, "project", "d", ".", "project directory")

	return cmd
}
