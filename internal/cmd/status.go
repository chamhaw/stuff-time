package cmd

import (
	"fmt"
	"os"
	"time"

	"stuff-time/internal/config"
	"stuff-time/internal/storage"

	"github.com/spf13/cobra"
)

var statusConfigPath string

func NewStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current status and statistics",
		RunE:  runStatus,
	}
	cmd.Flags().StringVarP(&statusConfigPath, "config", "c", "", "Path to config file")
	return cmd
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(statusConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	st, err := storage.NewStorage(cfg.Storage.DBPath, cfg.Storage.ReportsPath)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer st.Close()

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrow := today.AddDate(0, 0, 1)

	screenshots, err := st.QueryByDateRange(today, tomorrow)
	if err != nil {
		return fmt.Errorf("failed to query screenshots: %w", err)
	}

	summaries, err := st.QueryHourSummariesByDateRange(today, tomorrow)
	if err != nil {
		return fmt.Errorf("failed to query summaries: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Stuff-time Status\n")
	fmt.Fprintf(os.Stdout, "================\n\n")
	fmt.Fprintf(os.Stdout, "Today's Screenshots: %d\n", len(screenshots))
	fmt.Fprintf(os.Stdout, "Today's Hour Summaries: %d\n\n", len(summaries))

	if len(summaries) > 0 {
		fmt.Fprintf(os.Stdout, "Recent Hour Summaries:\n")
		for i, s := range summaries {
			if i >= 5 {
				break
			}
			fmt.Fprintf(os.Stdout, "  %s: %s\n", s.HourKey, truncate(s.Summary, 60))
		}
	}

	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

