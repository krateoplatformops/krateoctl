package ui

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

// ANSI Color Codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorGray   = "\033[90m"
)

// Level controls logger verbosity.
type Level int32

const (
	LevelInfo Level = iota
	LevelDebug
)

// Logger is a lightweight writer-aware logger inspired by kind's CLI logger.
type Logger struct {
	writer        io.Writer
	writerMu      sync.Mutex
	verbosity     int32
	bufferPool    *bufferPool
	isSmartWriter bool
}

// NewLogger builds a logger that writes to w using the provided verbosity level.
func NewLogger(w io.Writer, level Level) *Logger {
	l := &Logger{
		verbosity:  int32(level),
		bufferPool: newBufferPool(),
	}
	l.SetWriter(w)
	return l
}

// SetWriter updates the logger output writer and checks for terminal capabilities.
func (l *Logger) SetWriter(w io.Writer) {
	if w == nil {
		w = io.Discard
	}
	l.writerMu.Lock()
	defer l.writerMu.Unlock()

	l.writer = w

	// Check if the writer is our Spinner or wraps a terminal
	s, isSpinner := w.(*Spinner)
	var actualDest io.Writer = w
	if isSpinner {
		actualDest = s.writer
	}

	l.isSmartWriter = isSpinner || isSmartTerminal(actualDest)
}

// SetVerbosity changes the active verbosity level.
func (l *Logger) SetVerbosity(level Level) {
	atomic.StoreInt32(&l.verbosity, int32(level))
}

func (l *Logger) verbosityLevel() Level {
	return Level(atomic.LoadInt32(&l.verbosity))
}

// V returns an InfoLogger for the supplied level.
func (l *Logger) V(level Level) InfoLogger {
	return InfoLogger{
		logger:  l,
		level:   level,
		enabled: level <= l.verbosityLevel(),
	}
}

// Info prints an informational message with semantic coloring.
func (l *Logger) Info(format string, args ...any) {
	msg := formatMessage(format, args...)

	if l.isSmartWriter {
		if strings.HasPrefix(msg, "✓") {
			l.print(colorGreen, msg)
			return
		}
		if strings.HasPrefix(msg, "[SKIP]") {
			l.print(colorGray, msg)
			return
		}
	}
	l.print("", msg)
}

// Warn prints a warning message in yellow.
func (l *Logger) Warn(format string, args ...any) {
	l.print(colorYellow, formatMessage(format, args...))
}

// Error prints an error message in red.
func (l *Logger) Error(format string, args ...any) {
	l.print(colorRed, formatMessage(format, args...))
}

// Debug prints a debug message with a cyan header.
func (l *Logger) Debug(message string, args ...any) {
	if l.verbosityLevel() < LevelDebug {
		return
	}
	l.debug(formatMessage(message, args...))
}

func (l *Logger) print(color, message string) {
	buf := l.bufferPool.Get()
	defer l.bufferPool.Put(buf)

	if color != "" && l.isSmartWriter {
		buf.WriteString(color)
		buf.WriteString(message)
		buf.WriteString(colorReset)
	} else {
		buf.WriteString(message)
	}
	l.writeBuffer(buf)
}

func (l *Logger) debug(message string) {
	buf := l.bufferPool.Get()
	defer l.bufferPool.Put(buf)

	l.addDebugHeader(buf)
	buf.WriteString(message)
	l.writeBuffer(buf)
}

func (l *Logger) writeBuffer(buf *bytes.Buffer) {
	// Ensure trailing newline
	if buf.Len() == 0 || buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
	l.writerMu.Lock()
	defer l.writerMu.Unlock()
	_, _ = l.writer.Write(buf.Bytes())
}

func (l *Logger) addDebugHeader(buf *bytes.Buffer) {
	_, file, line, ok := runtime.Caller(3)
	if !ok {
		file = "???"
		line = 1
	} else {
		if slash := strings.LastIndex(file, "/"); slash >= 0 {
			path := file
			file = path[slash+1:]
			if dirsep := strings.LastIndex(path[:slash], "/"); dirsep >= 0 {
				file = path[dirsep+1:]
			}
		}
	}

	if l.isSmartWriter {
		buf.WriteString(colorCyan)
	}
	buf.WriteString("DEBUG [")
	buf.WriteString(file)
	buf.WriteByte(':')
	fmt.Fprintf(buf, "%d", line)
	buf.WriteString("] ")
	if l.isSmartWriter {
		buf.WriteString(colorReset)
	}
}

// isSmartTerminal checks if the writer is a TTY.
func isSmartTerminal(w io.Writer) bool {
	// Check for Fd() method (common for os.Stdout/Stderr)
	type fder interface {
		Fd() uintptr
	}

	if f, ok := w.(fder); ok {
		fd := f.Fd()
		return fd == os.Stdout.Fd() || fd == os.Stderr.Fd()
	}

	// Fallback check for standard streams
	if w == os.Stdout || w == os.Stderr {
		stat, err := os.Stdout.Stat()
		return err == nil && (stat.Mode()&os.ModeCharDevice) != 0
	}

	return false
}

// InfoLogger mirrors kind's InfoLogger implementation.
type InfoLogger struct {
	logger  *Logger
	level   Level
	enabled bool
}

// Enabled reports whether the logger should emit at this level.
func (i InfoLogger) Enabled() bool { return i.enabled }

// Info logs an info message if enabled.
func (i InfoLogger) Info(format string, args ...any) {
	if !i.enabled {
		return
	}
	// Route through the main logger to pick up semantic coloring (Green/Gray)
	if i.level >= LevelDebug {
		i.logger.Debug(format, args...)
		return
	}
	i.logger.Info(format, args...)
}

func formatMessage(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

type bufferPool struct {
	sync.Pool
}

func newBufferPool() *bufferPool {
	return &bufferPool{sync.Pool{New: func() any { return new(bytes.Buffer) }}}
}

func (b *bufferPool) Get() *bytes.Buffer {
	return b.Pool.Get().(*bytes.Buffer)
}

func (b *bufferPool) Put(buf *bytes.Buffer) {
	if buf.Len() > 256 {
		return
	}
	buf.Reset()
	b.Pool.Put(buf)
}
