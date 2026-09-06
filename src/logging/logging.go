package logging

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"redstone/modes"
)

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

type Logger struct {
	mu       sync.Mutex
	mode     modes.LogMode
	minLevel Level
	file     *os.File
	writers  []io.Writer
}

func New(mode modes.LogMode, filePath string) (*Logger, error) {
	l := &Logger{mode: mode, minLevel: LevelInfo}

	switch mode {
	case modes.LogModeSilent:
		return l, nil
	case modes.LogModeDebug:
		l.minLevel = LevelDebug
		l.writers = append(l.writers, os.Stdout)
	case modes.LogModeError:
		l.minLevel = LevelError
		l.writers = append(l.writers, os.Stdout)
	case modes.LogModeConsole:
		l.writers = append(l.writers, os.Stdout)
	case modes.LogModeFile:
		if err := l.openFile(filePath); err != nil {
			return nil, err
		}
	case modes.LogModeBoth:
		l.writers = append(l.writers, os.Stdout)
		if filePath != "" {
			if err := l.openFile(filePath); err != nil {
				return nil, err
			}
		}
	default:
		l.writers = append(l.writers, os.Stdout)
	}

	return l, nil
}

func (l *Logger) openFile(path string) error {
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("не удалось открыть файл логов %s: %w", path, err)
	}
	l.file = f
	l.writers = append(l.writers, f)
	return nil
}

func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

func (l *Logger) log(level Level, format string, args ...interface{}) {
	if l == nil || l.mode == modes.LogModeSilent || level < l.minLevel || len(l.writers) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	line := fmt.Sprintf("[%s] [%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), level, fmt.Sprintf(format, args...))
	for _, w := range l.writers {
		io.WriteString(w, line)
	}
}

func (l *Logger) Debug(format string, args ...interface{}) { l.log(LevelDebug, format, args...) }
func (l *Logger) Info(format string, args ...interface{})  { l.log(LevelInfo, format, args...) }
func (l *Logger) Warn(format string, args ...interface{})  { l.log(LevelWarn, format, args...) }
func (l *Logger) Error(format string, args ...interface{}) { l.log(LevelError, format, args...) }
