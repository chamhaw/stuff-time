package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"

	"stuff-time/internal/logger"
)

type Config struct {
	OpenAI      OpenAIConfig      `mapstructure:"openai"`
	Screenshot  ScreenshotConfig  `mapstructure:"screenshot"`
	Storage     StorageConfig     `mapstructure:"storage"`
	Evaluator   EvaluatorConfig   `mapstructure:"evaluator"`
	Performance PerformanceConfig `mapstructure:"performance"`
}

type OpenAIConfig struct {
	APIKey              string `mapstructure:"api_key"`
	BaseURL             string `mapstructure:"base_url"` // API base URL, defaults to OpenAI
	Model               string `mapstructure:"model"`    // Default model for screenshot analysis
	MaxCompletionTokens int    `mapstructure:"max_completion_tokens"`

	// Prompt scene paths (directories, not individual files)
	ScreenshotPath string `mapstructure:"screenshot_path"` // Path to screenshot analysis prompt scene directory
	SummaryPath    string `mapstructure:"summary_path"`    // Path to period summary prompt scene directory
	AnalysisPath   string `mapstructure:"analysis_path"`   // Path to behavior analysis prompt scene directory

	// Prompt content (loaded from files at runtime)
	PromptContent         string // Screenshot analysis prompt content
	SummaryPromptContent  string // Period summary prompt content
	AnalysisPromptContent string // Behavior analysis prompt content

	// Screenshot sub-prompts (loaded from screenshot_path directory)
	DesktopLockDetectionPromptContent string // Desktop/lock screen detection prompt content
	LockScreenDetectionPromptContent  string // Lock screen detection prompt content

	// Summary sub-prompts (loaded from summary_path directory)
	SummaryEnhancedContent      string // Enhanced summary prompt content
	SummaryContextPrefixContent string // Context prefix prompt content
	SummaryRollingContent       string // Rolling summary prompt content

	// Level-specific summary prompts (loaded from summary_path directory)
	FifteenminPromptContent string // 15-minute summary prompt content
	HourPromptContent       string // Hour summary prompt content
	DayPromptContent        string // Day summary prompt content
	WeekPromptContent       string // Week summary prompt content
	MonthPromptContent      string // Month summary prompt content
	QuarterPromptContent    string // Quarter summary prompt content
	YearPromptContent       string // Year summary prompt content

	// Summary configuration (frequent, simple task, cheaper model)
	SummaryModel string `mapstructure:"summary_model"` // Model for period summary generation

	// Analysis configuration (less frequent, complex task, stronger model)
	AnalysisModel string `mapstructure:"analysis_model"` // Model for deep behavior analysis
}

type EvaluatorConfig struct {
	EvaluationPath  string `mapstructure:"evaluation_path"`  // Path to evaluation prompt scene directory
	ImprovementPath string `mapstructure:"improvement_path"` // Path to improvement prompt scene directory

	// Evaluation prompt content (loaded from evaluation_path directory)
	EvaluationPromptContent        string // Evaluation main prompt content
	ReportContentContent           string // Report content prompt content
	ScreenshotSourceContent        string // Screenshot source prompt content
	ReportFormatContent            string // Report format prompt content
	ScreenshotSourceSectionContent string // Screenshot source section prompt content

	// Improvement prompt content (loaded from improvement_path directory)
	ImprovementPromptContent           string // Improvement main prompt content
	ImprovementScreenshotSourceContent string // Improvement screenshot source template content
}

type PerformanceConfig struct {
	MaxParallelFifteenmins     int `mapstructure:"max_parallel_fifteenmins"`
	MaxParallelHours           int `mapstructure:"max_parallel_hours"`
	MaxParallelDays            int `mapstructure:"max_parallel_days"`
	MaxParallelWeeks           int `mapstructure:"max_parallel_weeks"`
	MaxParallelMonths          int `mapstructure:"max_parallel_months"`
	MaxParallelQuarters        int `mapstructure:"max_parallel_quarters"`
	MaxParallelTreeAggregation int `mapstructure:"max_parallel_tree_aggregation"`
}

type ScreenshotConfig struct {
	Interval         string          `mapstructure:"interval"`
	Cron             string          `mapstructure:"cron"`
	StoragePath      string          `mapstructure:"storage_path"`
	ImageFormat      string          `mapstructure:"image_format"`
	AnalysisInterval string          `mapstructure:"analysis_interval"`
	AnalysisCron     string          `mapstructure:"analysis_cron"`
	SummaryPeriods   []string        `mapstructure:"summary_periods"`
	AnalysisWorkers  int             `mapstructure:"analysis_workers"` // Number of concurrent workers for analysis
	WorkHours        WorkHoursConfig `mapstructure:"work_hours"`       // Work hours configuration
	CleanupInterval  string          `mapstructure:"cleanup_interval"` // Interval for invalid reports cleanup
	CleanupCron      string          `mapstructure:"cleanup_cron"`     // Cron expression for invalid reports cleanup
}

type WorkHoursConfig struct {
	StartHour   int `mapstructure:"start_hour"`   // Work start hour (0-23)
	StartMinute int `mapstructure:"start_minute"` // Work start minute (0-59)
	EndHour     int `mapstructure:"end_hour"`     // Work end hour (0-23)
	EndMinute   int `mapstructure:"end_minute"`   // Work end minute (0-59)
}

// IsWorkTime checks if the given time is within work hours
func (w *WorkHoursConfig) IsWorkTime(t time.Time) bool {
	// If work hours are not configured (all zeros), consider all time as work time
	if w.StartHour == 0 && w.StartMinute == 0 && w.EndHour == 0 && w.EndMinute == 0 {
		return true
	}

	currentHour := t.Hour()
	currentMinute := t.Minute()

	// Convert to minutes for easier comparison
	currentMinutes := currentHour*60 + currentMinute
	startMinutes := w.StartHour*60 + w.StartMinute
	endMinutes := w.EndHour*60 + w.EndMinute

	// Handle case where work hours span midnight (e.g., 22:00 - 06:00)
	if endMinutes < startMinutes {
		// Work hours span midnight
		return currentMinutes >= startMinutes || currentMinutes < endMinutes
	}

	// Normal case: work hours within same day
	return currentMinutes >= startMinutes && currentMinutes < endMinutes
}

type StorageConfig struct {
	DBPath        string    `mapstructure:"db_path"`
	RetentionDays int       `mapstructure:"retention_days"`
	LogPath       string    `mapstructure:"log_path"`
	ReportsPath   string    `mapstructure:"reports_path"`
	Log           LogConfig `mapstructure:"log"`

	// 主观周期配置
	HourSegments    int    `mapstructure:"hour_segments"`     // 小时内分段数（默认4，即15分钟一段）
	DayWorkSegments int    `mapstructure:"day_work_segments"` // 日内工作段数（默认0，表示不使用工作段）
	MonthWeeks      string `mapstructure:"month_weeks"`       // 月内周数计算方式（默认"calendar"，可选"fixed"）
	YearQuarters    int    `mapstructure:"year_quarters"`     // 年内季度数（默认4）

	// 结构配置
	EnableNestedStructure bool `mapstructure:"enable_nested_structure"` // 启用层级嵌套结构（默认true）
	BackwardCompatible    bool `mapstructure:"backward_compatible"`     // 向后兼容模式（默认true，迁移完成后可设为false）
}

type LogConfig struct {
	Level        string `mapstructure:"level"`         // "debug", "info", "warn", "error"
	RotationTime string `mapstructure:"rotation_time"` // Time-based rotation interval (e.g., "1h", "24h")
	MaxSize      int    `mapstructure:"max_size"`      // Maximum size in megabytes before rotation
	MaxBackups   int    `mapstructure:"max_backups"`   // Maximum number of old log files to retain
	MaxAge       int    `mapstructure:"max_age"`       // Maximum number of days to retain old log files
	Compress     bool   `mapstructure:"compress"`      // Whether to compress rotated log files
}

// Validate 验证存储配置的有效性
func (c *StorageConfig) Validate() error {
	// 验证 HourSegments：必须能整除60
	if c.HourSegments <= 0 {
		return fmt.Errorf("hour_segments must be positive, got %d", c.HourSegments)
	}
	if 60%c.HourSegments != 0 {
		return fmt.Errorf("hour_segments must divide 60 evenly, got %d", c.HourSegments)
	}

	// 验证 DayWorkSegments：必须能整除24（0表示不使用）
	if c.DayWorkSegments < 0 {
		return fmt.Errorf("day_work_segments must be non-negative, got %d", c.DayWorkSegments)
	}
	if c.DayWorkSegments > 0 && 24%c.DayWorkSegments != 0 {
		return fmt.Errorf("day_work_segments must divide 24 evenly, got %d", c.DayWorkSegments)
	}

	// 验证 YearQuarters：必须能整除12
	if c.YearQuarters <= 0 {
		return fmt.Errorf("year_quarters must be positive, got %d", c.YearQuarters)
	}
	if 12%c.YearQuarters != 0 {
		return fmt.Errorf("year_quarters must divide 12 evenly, got %d", c.YearQuarters)
	}

	// 验证 MonthWeeks：必须为 "calendar" 或 "fixed"
	if c.MonthWeeks != "calendar" && c.MonthWeeks != "fixed" {
		return fmt.Errorf("month_weeks must be 'calendar' or 'fixed', got '%s'", c.MonthWeeks)
	}

	return nil
}

// ApplyDefaults 应用默认配置值
func (c *StorageConfig) ApplyDefaults() {
	if c.HourSegments == 0 {
		c.HourSegments = 4 // 默认4段，即15分钟一段
	}
	if c.DayWorkSegments == 0 {
		c.DayWorkSegments = 0 // 默认不使用工作段
	}
	if c.MonthWeeks == "" {
		c.MonthWeeks = "calendar" // 默认使用日历周
	}
	if c.YearQuarters == 0 {
		c.YearQuarters = 4 // 默认4个季度
	}
	// EnableNestedStructure 和 BackwardCompatible 默认为 false（零值），需要显式设置
}

var globalConfig *Config

func Load(configPath string) (*Config, error) {
	viper.SetConfigType("yaml")

	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		viper.SetConfigName("config")

		// Get executable directory for default config location
		execPath, err := os.Executable()
		if err == nil {
			execDir := filepath.Dir(execPath)
			viper.AddConfigPath(filepath.Join(execDir, "config"))
			viper.AddConfigPath(execDir)
		}

		// Also check current working directory (for development)
		viper.AddConfigPath("./config")
		viper.AddConfigPath(".")

		// Check user home directory (for user-specific config)
		if homeDir, err := os.UserHomeDir(); err == nil {
			viper.AddConfigPath(filepath.Join(homeDir, ".stuff-time"))
		}
	}

	viper.SetDefault("openai.base_url", "https://api.openai.com/v1")
	viper.SetDefault("openai.model", "gpt-4-vision-preview")
	viper.SetDefault("openai.max_completion_tokens", 500)
	viper.SetDefault("openai.screenshot_path", "prompts/screenshot")

	// Summary configuration (frequent, simple task, cheaper model)
	viper.SetDefault("openai.summary_model", "gpt-4o-mini")
	viper.SetDefault("openai.summary_path", "prompts/summary")

	// Analysis configuration (less frequent, complex task, stronger model)
	viper.SetDefault("openai.analysis_model", "gpt-4o")
	viper.SetDefault("openai.analysis_path", "prompts/analysis")

	// Evaluator configuration
	viper.SetDefault("evaluator.evaluation_path", "prompts/evaluation")
	viper.SetDefault("evaluator.improvement_path", "prompts/improvement")
	viper.SetDefault("screenshot.interval", "1m")
	viper.SetDefault("screenshot.storage_path", "./data/screenshots")
	viper.SetDefault("screenshot.image_format", "png")
	viper.SetDefault("screenshot.analysis_interval", "10m")
	viper.SetDefault("screenshot.summary_periods", []string{"fifteenmin", "hour", "day", "week", "month"})
	viper.SetDefault("screenshot.analysis_workers", 3) // Default to 3 concurrent workers
	viper.SetDefault("screenshot.work_hours.start_hour", 9)
	viper.SetDefault("screenshot.work_hours.start_minute", 30)
	viper.SetDefault("screenshot.work_hours.end_hour", 20)
	viper.SetDefault("screenshot.work_hours.end_minute", 0)
	viper.SetDefault("screenshot.cleanup_interval", "24h") // Default: cleanup once per day
	viper.SetDefault("screenshot.cleanup_cron", "")        // Default: use interval instead of cron
	viper.SetDefault("storage.db_path", "./data/db/stuff-time.db")
	viper.SetDefault("storage.reports_path", "./data/reports")
	viper.SetDefault("storage.retention_days", 30)
	viper.SetDefault("storage.log_path", "")
	viper.SetDefault("storage.log.level", "info")
	viper.SetDefault("storage.log.rotation_time", "1h") // Rotate logs every hour
	viper.SetDefault("storage.log.max_size", 100)       // 100MB
	viper.SetDefault("storage.log.max_backups", 3)      // Keep 3 old log files
	viper.SetDefault("storage.log.max_age", 28)         // Keep logs for 28 days
	viper.SetDefault("storage.log.compress", true)      // Compress rotated logs

	// 主观周期配置默认值
	viper.SetDefault("storage.hour_segments", 4)              // 默认4段，即15分钟一段
	viper.SetDefault("storage.day_work_segments", 0)          // 默认不使用工作段
	viper.SetDefault("storage.month_weeks", "calendar")       // 默认使用日历周
	viper.SetDefault("storage.year_quarters", 4)              // 默认4个季度
	viper.SetDefault("storage.enable_nested_structure", true) // 默认启用层级嵌套结构
	viper.SetDefault("storage.backward_compatible", true)     // 默认启用向后兼容模式

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if cfg.OpenAI.APIKey == "" {
		cfg.OpenAI.APIKey = os.Getenv("OPENAI_API_KEY")
	}

	// 应用存储配置默认值
	cfg.Storage.ApplyDefaults()

	// 验证存储配置
	if err := cfg.Storage.Validate(); err != nil {
		// 配置验证失败，记录警告并使用默认值
		fmt.Fprintf(os.Stderr, "Warning: Invalid storage configuration: %v. Using default values.\n", err)
		// 重置为默认配置
		cfg.Storage.HourSegments = 4
		cfg.Storage.DayWorkSegments = 0
		cfg.Storage.MonthWeeks = "calendar"
		cfg.Storage.YearQuarters = 4
		cfg.Storage.EnableNestedStructure = true
		cfg.Storage.BackwardCompatible = true
	}

	if err := normalizePaths(&cfg); err != nil {
		return nil, fmt.Errorf("failed to normalize paths: %w", err)
	}

	// Load prompt files
	configFileDir := ""
	if configPath != "" {
		configFileDir = filepath.Dir(configPath)
	} else {
		// Try to get config file directory from viper
		if configFile := viper.ConfigFileUsed(); configFile != "" {
			configFileDir = filepath.Dir(configFile)
		} else {
			// Fallback to default config directory
			baseDir, err := getBaseDirectory()
			if err == nil {
				configFileDir = filepath.Join(baseDir, "config")
			} else {
				configFileDir = "./config"
			}
		}
	}

	if err := loadPromptFiles(&cfg, configFileDir); err != nil {
		return nil, fmt.Errorf("failed to load prompt files: %w", err)
	}

	globalConfig = &cfg
	return &cfg, nil
}

func Get() *Config {
	if globalConfig == nil {
		panic("config not loaded")
	}
	return globalConfig
}

func (c *ScreenshotConfig) GetIntervalDuration() (time.Duration, error) {
	if c.Interval == "" {
		return 0, fmt.Errorf("interval not configured")
	}
	return time.ParseDuration(c.Interval)
}

func (c *ScreenshotConfig) GetAnalysisIntervalDuration() (time.Duration, error) {
	if c.AnalysisInterval == "" {
		return 0, fmt.Errorf("analysis interval not configured")
	}
	return time.ParseDuration(c.AnalysisInterval)
}

func (c *ScreenshotConfig) GetCleanupIntervalDuration() (time.Duration, error) {
	if c.CleanupInterval == "" {
		return 0, fmt.Errorf("cleanup interval not configured")
	}
	return time.ParseDuration(c.CleanupInterval)
}

func (c *ScreenshotConfig) EnsureStoragePath() error {
	return os.MkdirAll(c.StoragePath, 0755)
}

func (c *StorageConfig) EnsureDBPath() error {
	dir := filepath.Dir(c.DBPath)
	if dir != "." && dir != "" {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}

func (c *StorageConfig) EnsureReportsPath() error {
	if c.ReportsPath != "" {
		return os.MkdirAll(c.ReportsPath, 0755)
	}
	return nil
}

func normalizePaths(cfg *Config) error {
	// Use executable directory as base for relative paths, fallback to working directory
	baseDir, err := getBaseDirectory()
	if err != nil {
		// Fallback to working directory if executable path is not available
		baseDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get base directory: %w", err)
		}
	}

	if cfg.Screenshot.StoragePath != "" {
		if !filepath.IsAbs(cfg.Screenshot.StoragePath) {
			cfg.Screenshot.StoragePath = filepath.Join(baseDir, cfg.Screenshot.StoragePath)
		}
	}

	if cfg.Storage.LogPath == "" {
		cfg.Storage.LogPath = filepath.Join(baseDir, "stuff-time.log")
	} else if !filepath.IsAbs(cfg.Storage.LogPath) {
		cfg.Storage.LogPath = filepath.Join(baseDir, cfg.Storage.LogPath)
	}

	// If LogPath is a directory, append default filename
	if cfg.Storage.LogPath != "" {
		info, err := os.Stat(cfg.Storage.LogPath)
		if err == nil && info.IsDir() {
			// Path exists and is a directory, append default filename
			cfg.Storage.LogPath = filepath.Join(cfg.Storage.LogPath, "stuff-time.log")
		} else if err != nil && os.IsNotExist(err) {
			// Path doesn't exist, check if it looks like a directory (no extension)
			ext := filepath.Ext(cfg.Storage.LogPath)
			if ext == "" {
				// No extension, treat as directory and append default filename
				cfg.Storage.LogPath = filepath.Join(cfg.Storage.LogPath, "stuff-time.log")
			}
		}
	}

	if cfg.Storage.DBPath != "" && !filepath.IsAbs(cfg.Storage.DBPath) {
		cfg.Storage.DBPath = filepath.Join(baseDir, cfg.Storage.DBPath)
	}

	if cfg.Storage.ReportsPath != "" && !filepath.IsAbs(cfg.Storage.ReportsPath) {
		cfg.Storage.ReportsPath = filepath.Join(baseDir, cfg.Storage.ReportsPath)
	}

	// If log level is not set, use default
	if cfg.Storage.Log.Level == "" {
		cfg.Storage.Log.Level = "info"
	}

	// Initialize logger after config is loaded
	if err := initLogger(&cfg.Storage); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	return nil
}

// getBaseDirectory returns the base directory for resolving relative paths
// It tries to use the executable directory, falling back to working directory
// If executable is in bin/ directory, it walks up to find project root
func getBaseDirectory() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		// Fallback to working directory if executable path is not available
		return os.Getwd()
	}

	// Resolve symlinks to get the actual executable path
	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		// If symlink resolution fails, use the original path
		realPath = execPath
	}

	execDir := filepath.Dir(realPath)
	execDirName := filepath.Base(execDir)

	// If executable is in bin/ directory, try to find project root
	if execDirName == "bin" {
		// Walk up the directory tree to find project root
		// Project root is identified by presence of config/ directory
		currentDir := execDir
		for {
			parentDir := filepath.Dir(currentDir)

			// Stop if we've reached the filesystem root
			if parentDir == currentDir {
				break
			}

			// Check if this directory contains config/ directory
			configDirPath := filepath.Join(currentDir, "config")
			if info, err := os.Stat(configDirPath); err == nil && info.IsDir() {
				return currentDir, nil
			}

			currentDir = parentDir
		}
	}

	// If not in bin/ or project root not found, use executable directory
	return execDir, nil
}

// initLogger initializes the logger with storage config
func initLogger(storage *StorageConfig) error {
	return logger.Init(logger.LogConfig{
		Level:        storage.Log.Level,
		FilePath:     storage.LogPath,
		RotationTime: storage.Log.RotationTime,
		MaxSize:      storage.Log.MaxSize,
		MaxBackups:   storage.Log.MaxBackups,
		MaxAge:       storage.Log.MaxAge,
		Compress:     storage.Log.Compress,
	})
}

// loadPromptFiles loads prompt content from scene directories
// Supports both relative paths (relative to config file directory) and absolute paths
func loadPromptFiles(cfg *Config, configFileDir string) error {
	// Load screenshot prompts from screenshot scene directory
	if cfg.OpenAI.ScreenshotPath != "" {
		// Main screenshot analysis prompt
		content, err := loadPromptFromScene(cfg.OpenAI.ScreenshotPath, "screenshot.txt", configFileDir)
		if err != nil {
			return fmt.Errorf("failed to load screenshot prompt: %w", err)
		}
		cfg.OpenAI.PromptContent = content

		// Desktop/lock screen detection prompt (optional)
		if detectionPrompt, err := loadPromptFromScene(cfg.OpenAI.ScreenshotPath, "desktop-lock-detection.txt", configFileDir); err == nil {
			cfg.OpenAI.DesktopLockDetectionPromptContent = detectionPrompt
		}

		// Lock screen detection prompt (optional)
		if lockScreenPrompt, err := loadPromptFromScene(cfg.OpenAI.ScreenshotPath, "lock-screen-detection.txt", configFileDir); err == nil {
			cfg.OpenAI.LockScreenDetectionPromptContent = lockScreenPrompt
		}
	}

	// Load summary prompts from summary scene directory
	if cfg.OpenAI.SummaryPath != "" {
		// Main summary prompt
		content, err := loadPromptFromScene(cfg.OpenAI.SummaryPath, "main.txt", configFileDir)
		if err != nil {
			return fmt.Errorf("failed to load summary prompt: %w", err)
		}
		cfg.OpenAI.SummaryPromptContent = content

		// Enhanced summary prompt (optional)
		if enhanced, err := loadPromptFromScene(cfg.OpenAI.SummaryPath, "enhanced.txt", configFileDir); err == nil {
			cfg.OpenAI.SummaryEnhancedContent = enhanced
		}

		// Context prefix prompt (optional)
		if prefix, err := loadPromptFromScene(cfg.OpenAI.SummaryPath, "context-prefix.txt", configFileDir); err == nil {
			cfg.OpenAI.SummaryContextPrefixContent = prefix
		}

		// Rolling summary prompt (optional)
		if rolling, err := loadPromptFromScene(cfg.OpenAI.SummaryPath, "rolling.txt", configFileDir); err == nil {
			cfg.OpenAI.SummaryRollingContent = rolling
		}

		// Level-specific summary prompts (optional, fallback to main.txt if not found)
		if fifteenmin, err := loadPromptFromScene(cfg.OpenAI.SummaryPath, "fifteenmin.txt", configFileDir); err == nil {
			cfg.OpenAI.FifteenminPromptContent = fifteenmin
		}
		if hour, err := loadPromptFromScene(cfg.OpenAI.SummaryPath, "hour.txt", configFileDir); err == nil {
			cfg.OpenAI.HourPromptContent = hour
		}
		if day, err := loadPromptFromScene(cfg.OpenAI.SummaryPath, "day.txt", configFileDir); err == nil {
			cfg.OpenAI.DayPromptContent = day
		}
		if week, err := loadPromptFromScene(cfg.OpenAI.SummaryPath, "week.txt", configFileDir); err == nil {
			cfg.OpenAI.WeekPromptContent = week
		}
		if month, err := loadPromptFromScene(cfg.OpenAI.SummaryPath, "month.txt", configFileDir); err == nil {
			cfg.OpenAI.MonthPromptContent = month
		}
		if quarter, err := loadPromptFromScene(cfg.OpenAI.SummaryPath, "quarter.txt", configFileDir); err == nil {
			cfg.OpenAI.QuarterPromptContent = quarter
		}
		if year, err := loadPromptFromScene(cfg.OpenAI.SummaryPath, "year.txt", configFileDir); err == nil {
			cfg.OpenAI.YearPromptContent = year
		}
	}

	// Load analysis prompt (from analysis/analysis.txt or analysis.txt)
	if cfg.OpenAI.AnalysisPath != "" {
		content, err := loadPromptFromScene(cfg.OpenAI.AnalysisPath, "analysis.txt", configFileDir)
		if err != nil {
			return fmt.Errorf("failed to load analysis prompt: %w", err)
		}
		cfg.OpenAI.AnalysisPromptContent = content
	}

	// Load evaluation prompts from evaluation scene directory
	if cfg.Evaluator.EvaluationPath != "" {
		// Main evaluation prompt
		content, err := loadPromptFromScene(cfg.Evaluator.EvaluationPath, "main.txt", configFileDir)
		if err != nil {
			return fmt.Errorf("failed to load evaluation prompt: %w", err)
		}
		cfg.Evaluator.EvaluationPromptContent = content

		// Report content prompt (optional)
		if reportContent, err := loadPromptFromScene(cfg.Evaluator.EvaluationPath, "report-content.txt", configFileDir); err == nil {
			cfg.Evaluator.ReportContentContent = reportContent
		}

		// Screenshot source prompt (optional)
		if screenshotSource, err := loadPromptFromScene(cfg.Evaluator.EvaluationPath, "screenshot-source.txt", configFileDir); err == nil {
			cfg.Evaluator.ScreenshotSourceContent = screenshotSource
		}

		// Report format prompt (optional)
		if reportFormat, err := loadPromptFromScene(cfg.Evaluator.EvaluationPath, "report-format.txt", configFileDir); err == nil {
			cfg.Evaluator.ReportFormatContent = reportFormat
		}

		// Screenshot source section prompt (optional)
		if screenshotSection, err := loadPromptFromScene(cfg.Evaluator.EvaluationPath, "screenshot-source-section.txt", configFileDir); err == nil {
			cfg.Evaluator.ScreenshotSourceSectionContent = screenshotSection
		}
	}

	// Load improvement prompt from improvement scene directory
	if cfg.Evaluator.ImprovementPath != "" {
		// Main improvement prompt
		content, err := loadPromptFromScene(cfg.Evaluator.ImprovementPath, "main.txt", configFileDir)
		if err != nil {
			return fmt.Errorf("failed to load improvement prompt: %w", err)
		}
		cfg.Evaluator.ImprovementPromptContent = content

		// Screenshot source template (optional)
		if screenshotSource, err := loadPromptFromScene(cfg.Evaluator.ImprovementPath, "screenshot-source.txt", configFileDir); err == nil {
			cfg.Evaluator.ImprovementScreenshotSourceContent = screenshotSource
		}
	}

	return nil
}

// loadPromptFromScene loads a prompt file from a scene directory
// First tries to load from the scene directory, then tries the scene directory as a file
func loadPromptFromScene(scenePath, filename string, configFileDir string) (string, error) {
	// Try loading from scene directory
	sceneFilePath := filepath.Join(scenePath, filename)
	content, err := loadPromptFile(sceneFilePath, configFileDir)
	if err == nil {
		return content, nil
	}

	// If scene path is a file (not directory), try loading it directly
	// This supports backward compatibility with single-file prompts
	if filepath.Ext(scenePath) != "" {
		return loadPromptFile(scenePath, configFileDir)
	}

	return "", fmt.Errorf("failed to load prompt from scene %s: %w", scenePath, err)
}

// loadPromptFile loads a prompt file, supporting both relative and absolute paths
func loadPromptFile(promptPath string, configFileDir string) (string, error) {
	var filePath string

	if filepath.IsAbs(promptPath) {
		// Absolute path
		filePath = promptPath
	} else {
		// Relative path - try relative to config file directory first
		if configFileDir != "" {
			filePath = filepath.Join(configFileDir, promptPath)
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				// If not found, try relative to base directory
				baseDir, err := getBaseDirectory()
				if err == nil {
					filePath = filepath.Join(baseDir, "config", promptPath)
				} else {
					// Last resort: try current working directory
					filePath = filepath.Join("./config", promptPath)
				}
			}
		} else {
			// No config file directory, try base directory
			baseDir, err := getBaseDirectory()
			if err == nil {
				filePath = filepath.Join(baseDir, "config", promptPath)
			} else {
				filePath = filepath.Join("./config", promptPath)
			}
		}
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read prompt file %s: %w", filePath, err)
	}

	return string(content), nil
}
