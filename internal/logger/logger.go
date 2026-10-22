// Package logger provides structured logging with file rotation for the webserver stack.
package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Level represents log severity levels.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger is a simple, thread-safe logger with file rotation support.
type Logger struct {
	mu          sync.Mutex
	level       Level
	file        *os.File
	writer      io.Writer
	logDir      string
	name        string
	maxSizeBytes int64
	currentSize int64
}

const (
	// DefaultMaxLogSize is 10MB per log file before rotation.
	DefaultMaxLogSize = 10 * 1024 * 1024
	// MaxRotatedFiles is the number of rotated log files to keep.
	MaxRotatedFiles = 5
)

// New creates a new logger that writes to both a file and stdout.
func New(logDir, name string, level Level) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create log directory %s: %w", logDir, err)
	}

	logPath := filepath.Join(logDir, name+".log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("cannot open log file %s: %w", logPath, err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("cannot stat log file %s: %w", logPath, err)
	}

	l := &Logger{
		level:        level,
		file:         f,
		writer:       io.MultiWriter(f, os.Stdout),
		logDir:       logDir,
		name:         name,
		maxSizeBytes: DefaultMaxLogSize,
		currentSize:  info.Size(),
	}

	return l, nil
}

// NewStdout creates a logger that only writes to stdout (no file).
func NewStdout(level Level) *Logger {
	return &Logger{
		level:  level,
		writer: os.Stdout,
	}
}

// SetLevel changes the minimum log level.
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// Close closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// log writes a formatted log message at the given level.
func (l *Logger) log(level Level, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] [%s] %s\n", timestamp, level, msg)

	n, _ := fmt.Fprint(l.writer, line)
	l.currentSize += int64(n)

	// Check if rotation is needed
	if l.file != nil && l.currentSize >= l.maxSizeBytes {
		l.rotate()
	}
}

// rotate moves the current log file to a timestamped backup and opens a new one.
func (l *Logger) rotate() {
	if l.file == nil {
		return
	}

	l.file.Close()

	currentPath := filepath.Join(l.logDir, l.name+".log")
	rotatedPath := filepath.Join(l.logDir, fmt.Sprintf("%s.%s.log", l.name, time.Now().Format("20060102-150405")))
	os.Rename(currentPath, rotatedPath)

	// Remove old rotated files beyond MaxRotatedFiles
	l.cleanOldLogs()

	// Open a new log file
	f, err := os.OpenFile(currentPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// Fall back to stdout only
		l.file = nil
		l.writer = os.Stdout
		l.currentSize = 0
		return
	}

	l.file = f
	l.writer = io.MultiWriter(f, os.Stdout)
	l.currentSize = 0
}

// cleanOldLogs removes rotated log files beyond the retention limit.
func (l *Logger) cleanOldLogs() {
	pattern := filepath.Join(l.logDir, l.name+".*.log")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}

	if len(matches) <= MaxRotatedFiles {
		return
	}

	// Sort by name (which includes timestamp, so lexical sort = chronological)
	// Remove the oldest files
	toRemove := len(matches) - MaxRotatedFiles
	for i := 0; i < toRemove; i++ {
		os.Remove(matches[i])
	}
}

// Debug logs at debug level.
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(LevelDebug, format, args...)
}

// Info logs at info level.
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(LevelInfo, format, args...)
}

// Warn logs at warn level.
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(LevelWarn, format, args...)
}

// Error logs at error level.
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(LevelError, format, args...)
}

// Global logger instance
var globalLogger *Logger

// Init initializes the global logger.
func Init(logDir string, level Level) error {
	l, err := New(logDir, "webserver", level)
	if err != nil {
		return err
	}
	globalLogger = l
	return nil
}

// InitStdout initializes the global logger to stdout only.
func InitStdout(level Level) {
	globalLogger = NewStdout(level)
}

// G returns the global logger.
func G() *Logger {
	if globalLogger == nil {
		InitStdout(LevelInfo)
	}
	return globalLogger
}
