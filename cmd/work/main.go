package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/tscolari/work/internal/cli"
	"github.com/tscolari/work/internal/config"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:           "work",
		Short:         "Manage git worktree-based feature workspaces",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	root.AddCommand(startCmd(), endCmd(), cleanupBranchesCmd(), listCmd(), attachCmd())

	if err := root.Execute(); err != nil {
		var ce *cli.Error
		if errors.As(err, &ce) {
			fmt.Fprintln(os.Stderr, ce.Msg)
			os.Exit(ce.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func startCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start TICKET/kebab-description",
		Short: "Create a new feature workspace (branch + worktree + tmux session)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.RunWork(args, cmd.ErrOrStderr())
		},
	}
}

func endCmd() *cobra.Command {
	var force, dryRun bool

	cmd := &cobra.Command{
		Use:   "end",
		Short: "Tear down the current feature workspace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.RunWorkend(force, dryRun, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "skip the unmerged-commits check and delete the branch anyway")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would happen without doing anything")

	return cmd
}

func cleanupBranchesCmd() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "cleanup-branches",
		Short: "Delete local branches already merged into the default branch",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.RunCleanupBranches(yes, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")

	return cmd
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List active workspaces and their tmux session status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.RunList(cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

func attachCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attach <name>",
		Short: "Reconnect to a workspace, creating a tmux session if needed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cli.RunAttach(args[0], cmd.ErrOrStderr())
		},
	}

	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		cfg, err := config.Load()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		entries, err := os.ReadDir(cfg.WorktreeBase)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var names []string
		for _, e := range entries {
			if e.IsDir() {
				names = append(names, e.Name())
			}
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	}

	return cmd
}
