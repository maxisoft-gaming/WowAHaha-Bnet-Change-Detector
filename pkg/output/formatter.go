package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type OutputFormat string

const (
	FormatText OutputFormat = "text"
	FormatJSON OutputFormat = "json"
)

type TickResult struct {
	Timestamp       time.Time              `json:"timestamp"`
	Changed         bool                   `json:"changed"`
	ActionTriggered bool                   `json:"action_triggered"`
	TriggerReason   string                 `json:"trigger_reason,omitempty"`
	Regions         map[string]*RegionTick `json:"regions,omitempty"`
	Error           string                 `json:"error,omitempty"`
}

type RegionTick struct {
	LastModified *time.Time `json:"last_modified,omitempty"`
	ETag         string     `json:"etag,omitempty"`
	ContentLen   int64      `json:"content_length,omitempty"`
	Changed      bool       `json:"changed"`
}

type OutputHandler struct {
	mu           sync.Mutex
	Stdout       io.Writer
	Stderr       io.Writer
	Format       OutputFormat
	Verbose      bool
}

func NewOutputHandler(format OutputFormat, verbose bool) *OutputHandler {
	return &OutputHandler{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Format:  format,
		Verbose: verbose,
	}
}

// LogInfo logs informational diagnostic messages to stderr.
func (h *OutputHandler) LogInfo(format string, args ...interface{}) {
	h.logToStderr("INFO", format, args...)
}

// LogDebug logs debug diagnostic messages to stderr if verbose mode is enabled.
func (h *OutputHandler) LogDebug(format string, args ...interface{}) {
	if h.Verbose {
		h.logToStderr("DEBUG", format, args...)
	}
}

// LogWarn logs warning diagnostic messages to stderr.
func (h *OutputHandler) LogWarn(format string, args ...interface{}) {
	h.logToStderr("WARN", format, args...)
}

// LogError logs error diagnostic messages to stderr.
func (h *OutputHandler) LogError(format string, args ...interface{}) {
	h.logToStderr("ERROR", format, args...)
}

func (h *OutputHandler) logToStderr(level, formatStr string, args ...interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()

	msg := fmt.Sprintf(formatStr, args...)
	now := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(h.Stderr, "[%s] [%s] %s\n", now, level, msg)
}

// WriteTick outputs a single-line string to stdout representing one tick of the loop.
func (h *OutputHandler) WriteTick(res *TickResult) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if res.Timestamp.IsZero() {
		res.Timestamp = time.Now().UTC()
	}

	var line string
	if h.Format == FormatJSON {
		bytes, err := json.Marshal(res)
		if err != nil {
			return fmt.Errorf("failed to marshal tick JSON: %w", err)
		}
		line = string(bytes)
	} else {
		// Human-readable single-line format
		statusStr := "NO_CHANGE"
		if res.Changed {
			statusStr = "CHANGED"
		}
		triggerStr := "NO_TRIGGER"
		if res.ActionTriggered {
			triggerStr = fmt.Sprintf("TRIGGERED(%s)", res.TriggerReason)
		}

		regSummaries := make([]string, 0, len(res.Regions))
		for reg, rTick := range res.Regions {
			lmStr := "N/A"
			if rTick.LastModified != nil {
				lmStr = rTick.LastModified.UTC().Format(time.RFC3339)
			}
			chgStr := ""
			if rTick.Changed {
				chgStr = "*"
			}
			regSummaries = append(regSummaries, fmt.Sprintf("%s%s:LM=%s", reg, chgStr, lmStr))
		}

		errStr := ""
		if res.Error != "" {
			errStr = fmt.Sprintf(" ERROR=%q", res.Error)
		}

		line = fmt.Sprintf("%s status=%s action=%s regions=[%s]%s",
			res.Timestamp.UTC().Format(time.RFC3339),
			statusStr,
			triggerStr,
			strings.Join(regSummaries, ", "),
			errStr,
		)
	}

	// Guarantee single-line output without internal newlines
	line = strings.ReplaceAll(line, "\n", " ")
	line = strings.ReplaceAll(line, "\r", "")

	_, err := fmt.Fprintln(h.Stdout, line)
	return err
}
