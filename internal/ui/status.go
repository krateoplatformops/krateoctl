package ui

import "io"

// Status interfaces with Spinner to provide status updates similar to kind's status package.
type Status struct {
	spinner *Spinner
}

// NewStatus returns a status helper writing to w.
func NewStatus(w io.Writer) *Status {
	spinner := NewSpinner(w)
	spinner.Start()
	return &Status{spinner: spinner}
}

// End stops the spinner with a final message.
func (s *Status) End(message string) {
	s.spinner.Stop(message)
}

// Endf stops the spinner with formatted message.
func (s *Status) Endf(format string, args ...any) {
	s.spinner.Stopf(format, args...)
}

// Fail stops the spinner showing a failure message.
func (s *Status) Fail(message string) {
	s.spinner.Fail(message)
}

// Failf stops the spinner showing a formatted failure message.
func (s *Status) Failf(format string, args ...any) {
	s.spinner.Failf(format, args...)
}

// Start begins showing a new status message.
func (s *Status) Start(message string) {
	s.spinner.Restart(message)
}

// Startf begins showing a new formatted status message.
func (s *Status) Startf(format string, args ...any) {
	s.spinner.Restartf(format, args...)
}
