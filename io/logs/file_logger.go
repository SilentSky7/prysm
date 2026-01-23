package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/io/file"
)

// FileLogger allows logging to a specific file.
type FileLogger struct {
	filePath string
	mu       sync.Mutex
}

// NewFileLogger creates a new FileLogger instance.
func NewFileLogger(path string) *FileLogger {
	return &FileLogger{
		filePath: path,
	}
}

// Log writes a message to the log file with a timestamp.
// It creates the file if it doesn't exist, or appends to it if it does.
func (l *FileLogger) Log(format string, args ...interface{}) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Ensure the directory exists
	dir := filepath.Dir(l.filePath)
	if err := file.MkdirAll(dir); err != nil {
		return err
	}

	f, err := os.OpenFile(l.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, params.BeaconIoConfig().ReadWritePermissions)
	if err != nil {
		return err
	}
	defer f.Close()

	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	if _, err := f.WriteString(fmt.Sprintf("[%s] %s\n", timestamp, msg)); err != nil {
		return err
	}

	return nil
}

// Info writes a message to the log file using fmt.Sprint.
func (l *FileLogger) Info(args ...interface{}) error {
	return l.Log("%s", fmt.Sprint(args...))
}

// LogToFile writes a message to the specified log file with a timestamp.
// It acts as a global convenience function so you don't need to instantiate a FileLogger.
func LogToFile(path string, format string, args ...interface{}) error {
	return NewFileLogger(path).Log(format, args...)
}
