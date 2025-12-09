package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"stuff-time/internal/config"
	"stuff-time/internal/logger"
	"stuff-time/internal/scheduler"
	"stuff-time/internal/storage"
	"stuff-time/internal/task"
)

var configPath string

func NewStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the screenshot timer",
		RunE:  runStart,
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file")

	return cmd
}

func runStart(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configPath)
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

	var screenshotSched scheduler.Scheduler
	if cfg.Screenshot.Cron != "" {
		screenshotSched, err = scheduler.NewCronScheduler(cfg.Screenshot.Cron)
		if err != nil {
			return fmt.Errorf("failed to create screenshot cron scheduler: %w", err)
		}
	} else {
		interval, err := cfg.Screenshot.GetIntervalDuration()
		if err != nil {
			return fmt.Errorf("failed to parse screenshot interval: %w", err)
		}
		screenshotSched = scheduler.NewFixedRateScheduler(interval)
	}

	if err := screenshotSched.Start(executor.CaptureScreenshot); err != nil {
		return fmt.Errorf("failed to start screenshot scheduler: %w", err)
	}

	var analysisSched scheduler.Scheduler
	if cfg.Screenshot.AnalysisCron != "" {
		analysisSched, err = scheduler.NewCronScheduler(cfg.Screenshot.AnalysisCron)
		if err != nil {
			return fmt.Errorf("failed to create analysis cron scheduler: %w", err)
		}
	} else {
		interval, err := cfg.Screenshot.GetAnalysisIntervalDuration()
		if err != nil {
			return fmt.Errorf("failed to parse analysis interval: %w", err)
		}
		analysisSched = scheduler.NewFixedRateScheduler(interval)
	}

	analysisTask := func() error {
		if err := executor.BatchAnalyze(); err != nil {
			return err
		}
		
		// Check and fill missing summaries to reduce token consumption
		// This ensures all intermediate summaries (fifteenmin, halfhour, hour, etc.) are saved
		if err := executor.CheckAndFillMissingSummaries(7); err != nil {
			logger.GetLogger().Warnf("Failed to check and fill missing summaries: %v", err)
			// Continue even if this fails
		}
		
		return executor.GeneratePeriodSummary(false, false) // false: not manual, auto-generated
	}

	if err := analysisSched.Start(analysisTask); err != nil {
		return fmt.Errorf("failed to start analysis scheduler: %w", err)
	}

	// Setup cleanup scheduler for invalid reports
	var cleanupSched scheduler.Scheduler
	if cfg.Screenshot.CleanupCron != "" {
		cleanupSched, err = scheduler.NewCronScheduler(cfg.Screenshot.CleanupCron)
		if err != nil {
			return fmt.Errorf("failed to create cleanup cron scheduler: %w", err)
		}
	} else if cfg.Screenshot.CleanupInterval != "" {
		interval, err := cfg.Screenshot.GetCleanupIntervalDuration()
		if err != nil {
			return fmt.Errorf("failed to parse cleanup interval: %w", err)
		}
		cleanupSched = scheduler.NewFixedRateScheduler(interval)
	}

	if cleanupSched != nil {
		cleanupTask := func() error {
			return executor.CleanupInvalidReports()
		}

		if err := cleanupSched.Start(cleanupTask); err != nil {
			return fmt.Errorf("failed to start cleanup scheduler: %w", err)
		}
		logger.GetLogger().Infof("Cleanup scheduler started (interval: %s, cron: %s)", cfg.Screenshot.CleanupInterval, cfg.Screenshot.CleanupCron)
	}

	// Execute analysis immediately on startup
	logger.GetLogger().Info("Executing initial analysis on startup...")
	if err := analysisTask(); err != nil {
		logger.GetLogger().Warnf("Initial analysis failed: %v", err)
	} else {
		logger.GetLogger().Info("Initial analysis completed.")
	}

	logger.GetLogger().Info("Stuff-time started. Press Ctrl+C to stop.")
	logger.GetLogger().Infof("Screenshot interval: %s, Analysis interval: %s", cfg.Screenshot.Interval, cfg.Screenshot.AnalysisInterval)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.GetLogger().Info("Stopping...")
	if err := screenshotSched.Stop(); err != nil {
		return fmt.Errorf("failed to stop screenshot scheduler: %w", err)
	}
	if err := analysisSched.Stop(); err != nil {
		return fmt.Errorf("failed to stop analysis scheduler: %w", err)
	}
	if cleanupSched != nil {
		if err := cleanupSched.Stop(); err != nil {
			return fmt.Errorf("failed to stop cleanup scheduler: %w", err)
		}
	}
	logger.GetLogger().Info("Stopped.")

	return nil
}

