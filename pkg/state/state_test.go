package state

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/output"
)

func TestParseDateTime(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
		wantErr  bool
	}{
		{
			input:    "2026-07-22T18:30:00Z",
			expected: time.Date(2026, 7, 22, 18, 30, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			input:    "Wed, 22 Jul 2026 18:30:00 GMT",
			expected: time.Date(2026, 7, 22, 18, 30, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			input:    "2026-07-22 18:30:00",
			expected: time.Date(2026, 7, 22, 18, 30, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			input:    "1784745000",
			expected: time.Unix(1784745000, 0).UTC(),
			wantErr:  false,
		},
		{
			input:   "invalid-date",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseDateTime(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDateTime(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && !got.Equal(tt.expected) {
				t.Errorf("ParseDateTime(%q) = %v; want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestStateManager_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")

	logger := output.NewOutputHandler(output.FormatText, false)
	sm := NewStateManager(stateFile, nil, logger)

	lm := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)
	sm.UpdateRegion("eu", &RegionState{
		LastModified: &lm,
		ETag:         `"abc1234"`,
		ContentLen:   5000,
		LastChecked:  lm,
	})

	if err := sm.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Reload in a new manager
	sm2 := NewStateManager(stateFile, nil, logger)
	st := sm2.GetRegion("eu")
	if st == nil {
		t.Fatalf("Expected region eu to be loaded")
	}
	if st.ETag != `"abc1234"` || st.ContentLen != 5000 {
		t.Errorf("Unexpected region state: %+v", st)
	}
}

func getUnwritablePath() string {
	if runtime.GOOS == "windows" {
		return `Z:\non_existent_drive_x99\state.json`
	}
	return "/proc/nonexistent/state.json"
}

func TestStateManager_AutoEnabledSoftDisableOnSaveError(t *testing.T) {
	unwritablePath := getUnwritablePath()
	logger := output.NewOutputHandler(output.FormatText, false)

	// In auto-mode (explicit=false):
	sm := &StateManager{
		filePath:     unwritablePath,
		enabled:      true,
		userExplicit: false,
		logger:       logger,
	}

	err := sm.Save()
	if err != nil {
		t.Fatalf("In auto mode, save error should not be returned, got error: %v", err)
	}

	if sm.IsEnabled() {
		t.Errorf("Expected StateManager to soft-disable after save failure in auto mode")
	}
}

func TestStateManager_ExplicitUserEnabledFatalSaveError(t *testing.T) {
	unwritablePath := getUnwritablePath()
	logger := output.NewOutputHandler(output.FormatText, false)

	// In explicit mode (explicit=true):
	sm := &StateManager{
		filePath:     unwritablePath,
		enabled:      true,
		userExplicit: true,
		logger:       logger,
	}

	err := sm.Save()
	if err == nil {
		t.Fatalf("In explicit user mode, save error MUST return fatal error")
	}
}
