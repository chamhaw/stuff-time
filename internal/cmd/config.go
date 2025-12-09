package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"stuff-time/internal/config"
)

var configConfigPath string

func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show current configuration",
		RunE:  runConfig,
	}
	cmd.Flags().StringVarP(&configConfigPath, "config", "c", "", "Path to config file")
	return cmd
}

func runConfig(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Configuration\n")
	fmt.Fprintf(os.Stdout, "=============\n\n")
	fmt.Fprintf(os.Stdout, "OpenAI:\n")
	fmt.Fprintf(os.Stdout, "  Model: %s\n", cfg.OpenAI.Model)
	fmt.Fprintf(os.Stdout, "  Max Completion Tokens: %d\n", cfg.OpenAI.MaxCompletionTokens)
	fmt.Fprintf(os.Stdout, "  API Key: %s\n", maskAPIKey(cfg.OpenAI.APIKey))
	fmt.Fprintf(os.Stdout, "\nScreenshot:\n")
	fmt.Fprintf(os.Stdout, "  Interval: %s\n", cfg.Screenshot.Interval)
	fmt.Fprintf(os.Stdout, "  Cron: %s\n", cfg.Screenshot.Cron)
	fmt.Fprintf(os.Stdout, "  Storage Path: %s\n", cfg.Screenshot.StoragePath)
	fmt.Fprintf(os.Stdout, "  Image Format: %s\n", cfg.Screenshot.ImageFormat)
	fmt.Fprintf(os.Stdout, "  Analysis Interval: %s\n", cfg.Screenshot.AnalysisInterval)
	fmt.Fprintf(os.Stdout, "  Analysis Cron: %s\n", cfg.Screenshot.AnalysisCron)
	fmt.Fprintf(os.Stdout, "  Analysis Workers: %d\n", cfg.Screenshot.AnalysisWorkers)
	if len(cfg.Screenshot.SummaryPeriods) > 0 {
		fmt.Fprintf(os.Stdout, "  Summary Periods: %v\n", cfg.Screenshot.SummaryPeriods)
	} else {
		fmt.Fprintf(os.Stdout, "  Summary Periods: (default: halfhour, day, week, month)\n")
	}
	fmt.Fprintf(os.Stdout, "\nStorage:\n")
	fmt.Fprintf(os.Stdout, "  DB Path: %s\n", cfg.Storage.DBPath)
	fmt.Fprintf(os.Stdout, "  Retention Days: %d\n", cfg.Storage.RetentionDays)
	fmt.Fprintf(os.Stdout, "  Log Path: %s\n", cfg.Storage.LogPath)
	fmt.Fprintf(os.Stdout, "  Reports Path: %s\n", cfg.Storage.ReportsPath)
	fmt.Fprintf(os.Stdout, "\n  主观周期配置:\n")
	fmt.Fprintf(os.Stdout, "    Hour Segments: %d (每段 %d 分钟)\n", cfg.Storage.HourSegments, 60/cfg.Storage.HourSegments)
	if cfg.Storage.DayWorkSegments > 0 {
		fmt.Fprintf(os.Stdout, "    Day Work Segments: %d (每段 %d 小时)\n", cfg.Storage.DayWorkSegments, 24/cfg.Storage.DayWorkSegments)
	} else {
		fmt.Fprintf(os.Stdout, "    Day Work Segments: 0 (未启用)\n")
	}
	fmt.Fprintf(os.Stdout, "    Month Weeks: %s\n", cfg.Storage.MonthWeeks)
	fmt.Fprintf(os.Stdout, "    Year Quarters: %d\n", cfg.Storage.YearQuarters)
	fmt.Fprintf(os.Stdout, "\n  结构配置:\n")
	fmt.Fprintf(os.Stdout, "    Enable Nested Structure: %v\n", cfg.Storage.EnableNestedStructure)
	fmt.Fprintf(os.Stdout, "    Backward Compatible: %v\n", cfg.Storage.BackwardCompatible)

	return nil
}

func maskAPIKey(key string) string {
	if len(key) == 0 {
		return "(not set)"
	}
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-4:]
}
