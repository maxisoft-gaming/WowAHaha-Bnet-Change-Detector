package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestOutputHandler_WriteTickJSON(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	handler := &OutputHandler{
		Stdout:  stdout,
		Stderr:  stderr,
		Format:  FormatJSON,
		Verbose: true,
	}

	now := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)
	lm := time.Date(2026, 7, 22, 17, 30, 0, 0, time.UTC)

	res := &TickResult{
		Timestamp:       now,
		Changed:         true,
		ActionTriggered: true,
		TriggerReason:   "bnet_update",
		Regions: map[string]*RegionTick{
			"eu": {
				LastModified: &lm,
				ETag:         `"12345"`,
				ContentLen:   1024,
				Changed:      true,
			},
		},
	}

	err := handler.WriteTick(res)
	if err != nil {
		t.Fatalf("WriteTick failed: %v", err)
	}

	outStr := stdout.String()
	lines := strings.Split(strings.TrimSpace(outStr), "\n")
	if len(lines) != 1 {
		t.Fatalf("Expected exactly 1 line for JSON tick output, got %d lines", len(lines))
	}

	var parsed TickResult
	if err := json.Unmarshal([]byte(lines[0]), &parsed); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if !parsed.Changed || !parsed.ActionTriggered || parsed.TriggerReason != "bnet_update" {
		t.Errorf("Parsed result mismatch: %+v", parsed)
	}
}

func TestOutputHandler_WriteTickText(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	handler := &OutputHandler{
		Stdout: stdout,
		Stderr: stderr,
		Format: FormatText,
	}

	now := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)
	res := &TickResult{
		Timestamp:       now,
		Changed:         false,
		ActionTriggered: false,
	}

	if err := handler.WriteTick(res); err != nil {
		t.Fatalf("WriteTick failed: %v", err)
	}

	outStr := stdout.String()
	lines := strings.Split(strings.TrimSpace(outStr), "\n")
	if len(lines) != 1 {
		t.Fatalf("Expected exactly 1 line for Text tick output, got %d lines", len(lines))
	}

	if !strings.Contains(lines[0], "status=NO_CHANGE") {
		t.Errorf("Expected status=NO_CHANGE in text output, got: %s", lines[0])
	}
}
