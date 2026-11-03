package ui

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"time"
)

var asciiFrames = []string{"-", "\\", "|", "/"}
var brailleFrames = []string{
	"⢎⠁", "⠎⠁", "⠊⠁", "⠈⠁",
	"⢄⡱", "⢆⡱", "⢎⡱", "⢎⡰",
	"⢎⡠", "⢎⡀", "⠈⠁", "⠈⠑",
	"⠈⠱", "⠈⡱", "⢀⡱", "⢄⡱",
}

// Spinner renders animated progress feedback.
type Spinner struct {
	frameFormat string
	frames      []string
	suffix      string
	prefix      string
	ticker      *time.Ticker
	writer      io.Writer
	running     bool
	mu          sync.Mutex
	stop        chan struct{}
	stopped     chan struct{}
}

// NewSpinner creates a spinner that writes frames to w.
func NewSpinner(w io.Writer) *Spinner {
	frames := brailleFrames
	frameFormat := "\x1b[?7l\r\x1b[2K%s%s%s\x1b[?7h"
	if runtime.GOOS == "windows" {
		frames = asciiFrames
		frameFormat = "\r\x1b[2K%s%s%s"
	}
	return &Spinner{
		frameFormat: frameFormat,
		frames:      frames,
		writer:      w,
	}
}

// Start begins the spinner animation.
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stop = make(chan struct{})
	s.stopped = make(chan struct{})
	s.ticker = time.NewTicker(120 * time.Millisecond)
	go s.animate()
	s.mu.Unlock()
}

// Stop halts the spinner and prints a final message.
func (s *Spinner) Stop(message string) {
	s.stopSpinner()
	s.printLine(message)
}

// Stopf halts the spinner with formatted text.
func (s *Spinner) Stopf(format string, args ...any) {
	s.Stop(fmt.Sprintf(format, args...))
}

// Fail halts the spinner highlighting an error.
func (s *Spinner) Fail(message string) {
	s.stopSpinner()
	s.printLine(fmt.Sprintf("✗ %s", message))
}

// Failf halts with formatted failure text.
func (s *Spinner) Failf(format string, args ...any) {
	s.Fail(fmt.Sprintf(format, args...))
}

// Restart restarts the spinner with a new message.
func (s *Spinner) Restart(message string) {
	s.Stop("")
	s.SetSuffix(" " + message)
	s.Start()
}

// Restartf restarts with formatted output.
func (s *Spinner) Restartf(format string, args ...any) {
	s.Restart(fmt.Sprintf(format, args...))
}

// SetPrefix configures text before the spinner glyph.
func (s *Spinner) SetPrefix(prefix string) {
	s.mu.Lock()
	s.prefix = prefix
	s.mu.Unlock()
}

// SetSuffix configures text after the spinner glyph.
func (s *Spinner) SetSuffix(suffix string) {
	s.mu.Lock()
	s.suffix = suffix
	s.mu.Unlock()
}

// Write implements io.Writer ensuring spinner output remains tidy.
func (s *Spinner) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		if _, err := s.writer.Write([]byte("\r")); err != nil {
			return 0, err
		}
	}
	return s.writer.Write(p)
}

func (s *Spinner) animate() {
	frameIdx := 0
	for {
		select {
		case <-s.stop:
			s.mu.Lock()
			if s.ticker != nil {
				s.ticker.Stop()
				s.ticker = nil
			}
			s.running = false
			s.mu.Unlock()
			close(s.stopped)
			return
		case <-s.ticker.C:
			frame := s.frames[frameIdx%len(s.frames)]
			frameIdx++
			s.mu.Lock()
			fmt.Fprintf(s.writer, s.frameFormat, s.prefix, frame, s.suffix)
			s.mu.Unlock()
		}
	}
}

func (s *Spinner) stopSpinner() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	close(s.stop)
	stopped := s.stopped
	s.mu.Unlock()
	<-stopped
}

func (s *Spinner) printLine(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(message) == "" {
		fmt.Fprint(s.writer, "\r")
		return
	}
	fmt.Fprintf(s.writer, "\r%s\n", message)
}
