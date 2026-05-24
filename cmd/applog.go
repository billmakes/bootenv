package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const logPath = "/var/log/bootenv.log"

// logEvent appends one timestamped line to the bootenv log file.
// Format: <RFC3339 timestamp>  <full command line>
// Errors are non-fatal: a warning is printed to stderr but execution continues.
func logEvent(args []string) {
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not open log %s: %v\n", logPath, err)
		return
	}
	defer f.Close()

	line := fmt.Sprintf("%s  %s\n", time.Now().Format(time.RFC3339), strings.Join(args, " "))
	if _, err := fmt.Fprint(f, line); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write to log %s: %v\n", logPath, err)
	}
}
