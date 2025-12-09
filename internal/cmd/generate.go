package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"stuff-time/internal/config"
	"stuff-time/internal/storage"
	"stuff-time/internal/task"
)

var generateConfigPath string
var generatePeriod string
var generateDate string
var generateForceRebuild bool
var generateUpward bool
var generateRebuildFrom string

func NewGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate period summary reports based on actual data",
		Long:  "Generate period summary reports based on actual screenshot data. The time range is determined by the actual data available - if data exists up to a certain time, the report will cover that range.",
		RunE:  runGenerate,
	}

	cmd.Flags().StringVarP(&generateConfigPath, "config", "c", "", "Path to config file")
	cmd.Flags().StringVarP(&generatePeriod, "period", "p", "", "Specific period to generate (fifteenmin, hour, day, week, month, quarter, year). If not specified, generates all configured periods.")
	cmd.Flags().StringVarP(&generateDate, "date", "d", "", "Date for period generation (YYYY-MM-DD), defaults to today")
	cmd.Flags().BoolVarP(&generateForceRebuild, "force-rebuild", "f", false, "Force rebuild from screenshots: ignore existing lower-level summaries and regenerate from raw screenshots layer by layer")
	cmd.Flags().StringVarP(&generateRebuildFrom, "rebuild-from", "r", "", "Rebuild from specified level (fifteenmin, hour, work-segment, day, week, month, quarter). Keeps the specified level unchanged, but regenerates all higher levels. Mutually exclusive with --force-rebuild.")
	cmd.Flags().BoolVarP(&generateUpward, "upward", "u", false, "Generate all higher-level summaries from the specified period. All intermediate level reports will be updated.")

	return cmd
}

func runGenerate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(generateConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.Screenshot.EnsureStoragePath(); err != nil {
		return fmt.Errorf("failed to create storage path: %w", err)
	}

	if err := cfg.Storage.EnsureDBPath(); err != nil {
		return fmt.Errorf("failed to create db path: %w", err)
	}

	if err := cfg.Storage.EnsureReportsPath(); err != nil {
		return fmt.Errorf("failed to create reports path: %w", err)
	}

	st, err := storage.NewStorage(cfg.Storage.DBPath, cfg.Storage.ReportsPath)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}
	defer st.Close()

	executor, err := task.NewExecutor(cfg, st)
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}

	// Validate mutually exclusive flags
	if generateForceRebuild && generateRebuildFrom != "" {
		return fmt.Errorf("--force-rebuild and --rebuild-from are mutually exclusive")
	}
	
	// Validate rebuild-from level if specified
	if generateRebuildFrom != "" {
		validRebuildLevels := map[string]bool{
			"fifteenmin": true, "hour": true, "work-segment": true, 
			"day": true, "week": true, "month": true, "quarter": true,
		}
		if !validRebuildLevels[generateRebuildFrom] {
			return fmt.Errorf("invalid --rebuild-from level: %s (must be: fifteenmin, hour, work-segment, day, week, month, quarter)", generateRebuildFrom)
		}
	}
	
	// Generate period summaries based on actual data
	if generatePeriod != "" {
		// Generate specific period
		if generateForceRebuild {
			fmt.Fprintf(os.Stdout, "Generating %s summary report (force rebuild from screenshots)...\n", generatePeriod)
		} else {
			fmt.Fprintf(os.Stdout, "Generating %s summary report...\n", generatePeriod)
		}
		if err := executor.GenerateSinglePeriodSummary(generatePeriod, generateDate, generateForceRebuild); err != nil {
			return fmt.Errorf("failed to generate %s summary: %w", generatePeriod, err)
		}
		fmt.Fprintf(os.Stdout, "%s summary report generated successfully.\n", generatePeriod)
		
		// If --upward flag is set, generate all higher-level summaries
		if generateUpward {
			fmt.Fprintf(os.Stdout, "Generating all higher-level summaries from %s (upward aggregation)...\n", generatePeriod)
			if err := executor.GenerateHigherLevelSummaries(generatePeriod, generateDate, generateForceRebuild); err != nil {
				return fmt.Errorf("failed to generate higher-level summaries from %s: %w", generatePeriod, err)
			}
			fmt.Fprintf(os.Stdout, "All higher-level summaries generated successfully.\n")
		}
	} else {
		// Generate all configured periods
		if generateForceRebuild {
			fmt.Fprintf(os.Stdout, "Generating period summary reports for all configured periods (force rebuild from screenshots)...\n")
		} else {
			fmt.Fprintf(os.Stdout, "Generating period summary reports for all configured periods...\n")
		}
		if err := executor.GeneratePeriodSummary(generateForceRebuild, true); err != nil { // true: manual generation
			return fmt.Errorf("failed to generate period summaries: %w", err)
		}
		fmt.Fprintf(os.Stdout, "All period summary reports generated successfully.\n")
	}

	return nil
}

