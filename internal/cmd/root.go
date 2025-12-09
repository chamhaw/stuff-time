package cmd

import (
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "stuff-time",
		Short: "Stuff time - screenshot-based time tracking agent",
		Long:  "A time tracking agent that captures screenshots and analyzes them using LLM",
	}

	rootCmd.AddCommand(NewStartCmd())
	rootCmd.AddCommand(NewStatusCmd())
	rootCmd.AddCommand(NewQueryCmd())
	rootCmd.AddCommand(NewConfigCmd())
	rootCmd.AddCommand(NewCleanupCmd())
	rootCmd.AddCommand(NewSummaryCmd())
	rootCmd.AddCommand(NewDaemonCmd())
	rootCmd.AddCommand(NewTriggerCmd())            // Debug command
	rootCmd.AddCommand(NewGenerateCmd())           // User command for generating reports
	rootCmd.AddCommand(NewRebuildCmd())            // Rebuild database from screenshot directory
	rootCmd.AddCommand(NewEvaluateCmd())           // Evaluate period report quality
	rootCmd.AddCommand(NewImproveCmd())            // Improve period report based on evaluation feedback
	rootCmd.AddCommand(NewValidateCmd())           // Validate consistency between database and files
	rootCmd.AddCommand(NewScanInvalidReportsCmd()) // Scan and detect invalid report files

	return rootCmd
}
