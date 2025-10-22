package users

import (
	"fmt"
	"os"
)

// WriteToEnv appends the given line to the specified file, or creates it if it doesn't exist
func WriteToEnv(filename, line string) error {
	// Open file in append mode, create if not exists, with 0644 permissions
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer f.Close()

	// Add newline if needed
	if _, err := f.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("cannot write to file: %w", err)
	}

	return nil
}
