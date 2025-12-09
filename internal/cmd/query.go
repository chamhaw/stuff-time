package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"stuff-time/internal/config"
	"stuff-time/internal/storage"
)

var queryDate string
var queryHour string
var queryConfigPath string

func NewQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query completed historical reports",
		Long:  "Query and view completed historical reports that have already been generated.",
		RunE:  runQuery,
	}

	cmd.Flags().StringVarP(&queryDate, "date", "d", "", "Query date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&queryHour, "hour", "", "Query hour (0-23)")
	cmd.Flags().StringVarP(&queryConfigPath, "config", "c", "", "Path to config file")

	return cmd
}

func runQuery(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(queryConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	st, err := storage.NewStorage(cfg.Storage.DBPath, cfg.Storage.ReportsPath)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer st.Close()

	var start, end time.Time

	if queryDate != "" {
		date, err := time.Parse("2006-01-02", queryDate)
		if err != nil {
			return fmt.Errorf("invalid date format: %w", err)
		}

		if queryHour != "" {
			hour := 0
			if _, err := fmt.Sscanf(queryHour, "%d", &hour); err != nil || hour < 0 || hour > 23 {
				return fmt.Errorf("invalid hour: must be 0-23")
			}
			start = time.Date(date.Year(), date.Month(), date.Day(), hour, 0, 0, 0, date.Location())
			end = start.Add(time.Hour)
		} else {
			start = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
			end = start.AddDate(0, 0, 1)
		}
	} else {
		now := time.Now()
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 0, 1)
	}

	if queryHour != "" && queryDate == "" {
		return fmt.Errorf("hour requires date to be specified")
	}

	if queryHour != "" {
		hourKey := start.Format("2006-01-02-15")
		summary, err := st.GetHourSummary(hourKey)
		if err != nil {
			return fmt.Errorf("failed to get hour summary: %w", err)
		}

		if summary != nil {
			fmt.Fprintf(os.Stdout, "Hour Summary: %s\n", hourKey)
			fmt.Fprintf(os.Stdout, "================\n\n")
			fmt.Fprintf(os.Stdout, "%s\n\n", summary.Summary)

			screenshots, err := st.GetScreenshotsByHourKey(hourKey)
			if err != nil {
				return fmt.Errorf("failed to get screenshots: %w", err)
			}

			fmt.Fprintf(os.Stdout, "Screenshots (%d):\n", len(screenshots))
			for i, s := range screenshots {
				fmt.Fprintf(os.Stdout, "\n[%d] %s - %s\n", i+1, s.Timestamp.Format("15:04:05"), s.ImagePath)
				if s.Analysis != "" && !strings.HasPrefix(s.Analysis, "Analysis failed") {
					fmt.Fprintf(os.Stdout, "    Analysis: %s\n", s.Analysis)
				} else if s.Analysis != "" {
					fmt.Fprintf(os.Stdout, "    Analysis: (Failed)\n")
				} else {
					fmt.Fprintf(os.Stdout, "    Analysis: (Not analyzed yet)\n")
				}
			}
		} else {
			fmt.Fprintf(os.Stdout, "No data found for %s\n", hourKey)
		}
	} else {
		summaries, err := st.QueryHourSummariesByDateRange(start, end)
		if err != nil {
			return fmt.Errorf("failed to query summaries: %w", err)
		}

		if len(summaries) == 0 {
			fmt.Fprintf(os.Stdout, "No data found for %s\n", start.Format("2006-01-02"))
			return nil
		}

		fmt.Fprintf(os.Stdout, "Hour Summaries for %s\n", start.Format("2006-01-02"))
		fmt.Fprintf(os.Stdout, "================\n\n")

		for _, s := range summaries {
			fmt.Fprintf(os.Stdout, "%s: %s\n\n", s.HourKey, s.Summary)
		}
	}

	return nil
}
