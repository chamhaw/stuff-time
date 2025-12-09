package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"stuff-time/internal/analyzer"
	"stuff-time/internal/config"
	"stuff-time/internal/storage"
)

var (
	rebuildConfigPath string
	rebuildYes        bool
)

func NewRebuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild screenshot database by scanning screenshot directory",
		Long: `Rebuild the screenshot database by scanning the screenshot directory and importing all found screenshots.
This will clear all screenshot data in the database and rebuild it from the file system.
All screenshots will need to be re-analyzed after rebuilding.

Note: This command rebuilds screenshot metadata (screenshots table), not period summaries.
To rebuild period summaries from report files, use 'validate --rebuild-db' instead.`,
		RunE: runRebuild,
	}
	cmd.Flags().StringVarP(&rebuildConfigPath, "config", "c", "", "Path to config file")
	cmd.Flags().BoolVarP(&rebuildYes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func runRebuild(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(rebuildConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	st, err := storage.NewStorage(cfg.Storage.DBPath, cfg.Storage.ReportsPath)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer st.Close()

	fmt.Fprintf(os.Stdout, "Rebuilding database from screenshot directory: %s\n", cfg.Screenshot.StoragePath)
	if !rebuildYes {
		fmt.Fprintf(os.Stdout, "WARNING: This will clear all existing data in the database.\n")
		fmt.Fprintf(os.Stdout, "Use --yes flag to skip this warning.\n")
		return fmt.Errorf("rebuild cancelled (use --yes to confirm)")
	}

	fmt.Fprintf(os.Stdout, "Scanning directory and importing screenshots...\n")
	
	// Create analyzer for lock screen detection if API key is configured
	var lockScreenDetector storage.LockScreenDetector
	if cfg.OpenAI.APIKey != "" {
		openAI := analyzer.NewOpenAI(
			cfg.OpenAI.APIKey,
			cfg.OpenAI.BaseURL,
			cfg.OpenAI.Model,
			cfg.OpenAI.MaxCompletionTokens,
			cfg.OpenAI.PromptContent,
			cfg.OpenAI.DesktopLockDetectionPromptContent,
			cfg.OpenAI.LockScreenDetectionPromptContent,
			cfg.OpenAI.SummaryModel,
			cfg.OpenAI.SummaryPromptContent,
			cfg.OpenAI.SummaryEnhancedContent,
			cfg.OpenAI.SummaryContextPrefixContent,
			cfg.OpenAI.SummaryRollingContent,
			cfg.OpenAI.AnalysisModel,
			cfg.OpenAI.AnalysisPromptContent,
		)
		lockScreenDetector = openAI.IsLockScreen
		fmt.Fprintf(os.Stdout, "Lock screen detection enabled (using LLM analysis)\n")
	} else {
		fmt.Fprintf(os.Stdout, "WARNING: OpenAI API key not configured, lock screen detection disabled\n")
	}
	
	count, err := st.RebuildFromDirectory(cfg.Screenshot.StoragePath, lockScreenDetector)
	if err != nil {
		return fmt.Errorf("failed to rebuild database: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Successfully imported %d screenshot(s).\n", count)
	fmt.Fprintf(os.Stdout, "Database rebuild completed. Screenshots will be analyzed on the next analysis cycle.\n")

	return nil
}

