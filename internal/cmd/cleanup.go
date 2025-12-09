package cmd

import (
	"fmt"
	"os"

	"stuff-time/internal/config"
	"stuff-time/internal/storage"

	"github.com/spf13/cobra"
)

var cleanupConfigPath string

func NewCleanupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Clean up old records and screenshots",
		RunE:  runCleanup,
	}
	cmd.Flags().StringVarP(&cleanupConfigPath, "config", "c", "", "Path to config file")
	return cmd
}

func runCleanup(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cleanupConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	st, err := storage.NewStorage(cfg.Storage.DBPath, cfg.Storage.ReportsPath)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer st.Close()

	if err := st.CleanupOldRecords(cfg.Storage.RetentionDays); err != nil {
		return fmt.Errorf("failed to cleanup old records: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Cleanup completed. Records older than %d days have been removed.\n", cfg.Storage.RetentionDays)
	return nil
}

