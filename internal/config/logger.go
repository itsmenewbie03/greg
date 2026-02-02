package config

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

// InitLogger initializes the application logger based on configuration
func InitLogger(cfg *LoggingConfig) (*slog.Logger, error) {
	// Parse log level
	level := parseLogLevel(cfg.Level)

	// If file is empty, try to use default
	if cfg.File == "" {
		cfg.File = filepath.Join(getStateDir(), "greg", "greg.log")
	}

	// Create log file directory if it doesn't exist
	if cfg.File != "" {
		logDir := filepath.Dir(cfg.File)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
	}

	// Configure log rotation
	var writer io.Writer
	if cfg.File != "" {
		writer = &lumberjack.Logger{
			Filename:   cfg.File,
			MaxSize:    cfg.MaxSize, // megabytes
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge, // days
			Compress:   cfg.Compress,
		}
	} else {
		writer = os.Stderr
	}

	// Create handler based on format
	var handler slog.Handler
	handlerOpts := &slog.HandlerOptions{
		Level: level,
	}

	switch strings.ToLower(cfg.Format) {
	case "json":
		handler = slog.NewJSONHandler(writer, handlerOpts)
	default:
		// For text format, we'll use a colored handler if enabled and outputting to console
		isConsole := cfg.File == "" // Only apply coloring when logging to console, not file
		if cfg.Color && isConsole {
			handler = NewColoredTextHandler(writer, handlerOpts)
		} else {
			handler = slog.NewTextHandler(writer, handlerOpts)
		}
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger, nil
}

// ColoredTextHandler wraps slog.TextHandler to add colors for console output
type ColoredTextHandler struct {
	handler slog.Handler
	writer  io.Writer
	opts    *slog.HandlerOptions
}

// NewColoredTextHandler creates a new handler that adds colors for console output
func NewColoredTextHandler(w io.Writer, opts *slog.HandlerOptions) *ColoredTextHandler {
	// Create a text handler that writes to a buffer
	textHandler := slog.NewTextHandler(w, opts)
	return &ColoredTextHandler{
		handler: textHandler,
		writer:  w,
		opts:    opts,
	}
}

// Handle implements slog.Handler interface
func (h *ColoredTextHandler) Handle(ctx context.Context, r slog.Record) error {
	// Get the original text representation
	var buf strings.Builder
	textHandler := slog.NewTextHandler(&buf, h.opts)
	err := textHandler.Handle(ctx, r)
	if err != nil {
		return err
	}

	// Get the level to determine color
	level := r.Level.String()

	// Apply color based on level
	coloredLine := h.addColor(buf.String(), level)

	// Write the colored line to the actual writer
	_, err = h.writer.Write([]byte(coloredLine))
	return err
}

// addColor applies ANSI color codes based on log level
func (h *ColoredTextHandler) addColor(line, level string) string {
	var colorFunc func(string) string

	// Determine color based on log level
	switch level {
	case "DEBUG":
		// Gray for debug
		colorFunc = func(s string) string {
			return fmt.Sprintf("\033[90m%s\033[0m", s) // bright black/gray
		}
	case "INFO":
		// Green for info
		colorFunc = func(s string) string {
			return fmt.Sprintf("\033[32m%s\033[0m", s) // green
		}
	case "WARN":
		// Yellow for warning
		colorFunc = func(s string) string {
			return fmt.Sprintf("\033[33m%s\033[0m", s) // yellow
		}
	case "ERROR":
		// Red for error
		colorFunc = func(s string) string {
			return fmt.Sprintf("\033[31m%s\033[0m", s) // red
		}
	default:
		return line // Return unchanged if no match
	}

	// Colorize the first word (typically the level) in the log line
	parts := strings.SplitN(line, " ", 2)
	if len(parts) >= 2 {
		coloredPart := colorFunc(parts[0])
		return coloredPart + " " + parts[1]
	}
	return colorFunc(line)
}

// WithAttrs implements slog.Handler interface
func (h *ColoredTextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ColoredTextHandler{
		handler: h.handler.WithAttrs(attrs),
		writer:  h.writer,
		opts:    h.opts,
	}
}

// WithGroup implements slog.Handler interface
func (h *ColoredTextHandler) WithGroup(name string) slog.Handler {
	return &ColoredTextHandler{
		handler: h.handler.WithGroup(name),
		writer:  h.writer,
		opts:    h.opts,
	}
}

// Enabled implements slog.Handler interface
func (h *ColoredTextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// parseLogLevel parses a log level string
func parseLogLevel(levelStr string) slog.Level {
	switch strings.ToLower(levelStr) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
