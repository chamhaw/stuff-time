package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/DeRuina/timberjack"
	"github.com/sirupsen/logrus"
)

var (
	// Logger is the global logger instance
	Logger *logrus.Logger
	// initialized tracks if logger has been initialized
	initialized bool
)

// LogConfig holds configuration for logging
type LogConfig struct {
	Level          string // "debug", "info", "warn", "error"
	FilePath       string // Path to log file
	RotationTime   string // Time-based rotation interval (e.g., "1h", "24h")
	MaxSize        int    // Maximum size in megabytes before rotation
	MaxBackups     int    // Maximum number of old log files to retain
	MaxAge         int    // Maximum number of days to retain old log files
	Compress       bool   // Whether to compress rotated log files
}

// Init initializes the global logger with the given configuration
func Init(config LogConfig) error {
	// If already initialized, just return to prevent duplicate initialization
	if initialized && Logger != nil {
		return nil
	}
	
	// If Logger exists but not initialized (from GetLogger fallback), reuse it
	if Logger == nil {
		Logger = logrus.New()
	}
	
	// Disable default output to prevent duplicate logs
	Logger.SetOutput(io.Discard)

	// Set log level
	level, err := logrus.ParseLevel(config.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	Logger.SetLevel(level)

	// Set formatter
	Logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
		DisableColors:   true,
	})

	// Set output - collect all writers first
	var writers []io.Writer

	// Write to stdout only if it's not redirected to a file (avoid duplication in daemon mode)
	if !isStdoutRedirectedToFile() {
		writers = append(writers, os.Stdout)
	}

	// If log file is specified, add file writer with time and size rotation
	if config.FilePath != "" {
		// Ensure log directory exists
		dir := filepath.Dir(config.FilePath)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
		}

		// Set defaults if not specified
		maxSize := config.MaxSize
		if maxSize == 0 {
			maxSize = 100 // Default: 100MB
		}
		maxBackups := config.MaxBackups
		if maxBackups == 0 {
			maxBackups = 3 // Default: keep 3 old log files
		}
		maxAge := config.MaxAge
		if maxAge == 0 {
			maxAge = 28 // Default: keep logs for 28 days
		}

		// Parse rotation time, default to 1 hour
		rotationDuration := time.Hour
		if config.RotationTime != "" {
			var err error
			rotationDuration, err = time.ParseDuration(config.RotationTime)
			if err != nil {
				return fmt.Errorf("invalid rotation_time: %w", err)
			}
		}

		// Determine compression type
		compression := ""
		if config.Compress {
			compression = "gzip"
		}

		// Create timberjack logger with both time and size rotation
		fileWriter := &timberjack.Logger{
			Filename:         config.FilePath,
			MaxSize:          maxSize,              // megabytes
			MaxBackups:       maxBackups,           // number of backups
			MaxAge:           maxAge,              // days
			RotationInterval: rotationDuration,    // time-based rotation
			Compression:      compression,         // compression type
			LocalTime:        true,                 // use local time
		}
		writers = append(writers, fileWriter)
	}

	// Use multi-writer to write to both stdout and file
	Logger.SetOutput(io.MultiWriter(writers...))
	
	initialized = true

	return nil
}

// isStdoutRedirectedToFile checks if stdout is redirected to a regular file
func isStdoutRedirectedToFile() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	// If stdout is a regular file (not terminal/pipe), it's redirected
	return (stat.Mode() & os.ModeCharDevice) == 0 && stat.Mode().IsRegular()
}

// GetLogger returns the global logger instance
func GetLogger() *logrus.Logger {
	if Logger == nil {
		// Fallback: initialize with defaults if not initialized
		// But mark as not fully initialized so Init() can update it
		Logger = logrus.New()
		Logger.SetOutput(io.Discard) // Prevent default output
		Logger.SetLevel(logrus.InfoLevel)
		Logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
			DisableColors:   true,
		})
		// Don't set initialized = true, so Init() can still configure it properly
	}
	return Logger
}

