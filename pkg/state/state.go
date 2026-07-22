package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/output"
)

type RegionState struct {
	LastModified *time.Time `json:"last_modified,omitempty"`
	ETag         string     `json:"etag,omitempty"`
	ContentLen   int64      `json:"content_length,omitempty"`
	LastChecked  time.Time  `json:"last_checked"`
}

type AppState struct {
	LastWorkflowDispatch *time.Time              `json:"last_workflow_dispatch,omitempty"`
	Regions              map[string]*RegionState `json:"regions"`
}

type StateManager struct {
	mu           sync.RWMutex
	filePath     string
	enabled      bool
	userExplicit bool
	data         AppState
	logger       *output.OutputHandler
}

// DefaultStatePath returns the default state file path depending on OS.
func DefaultStatePath() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	// On Linux / POSIX systems, try /var/run/bnet-change-detector/state.json or /tmp
	if _, err := os.Stat("/var/run"); err == nil {
		return "/var/run/bnet-change-detector-state.json"
	}
	return filepath.Join(os.TempDir(), "bnet-change-detector-state.json")
}

// NewStateManager initializes a StateManager respecting OS defaults and user explicit flags.
func NewStateManager(path string, enableFlag *bool, logger *output.OutputHandler) *StateManager {
	var enabled bool
	var explicit bool

	if enableFlag != nil {
		enabled = *enableFlag
		explicit = true
	} else if path != "" {
		enabled = true
		explicit = true
	} else {
		// OS defaults
		if runtime.GOOS == "windows" {
			enabled = false
			path = ""
		} else {
			enabled = true
			path = DefaultStatePath()
		}
		explicit = false
	}

	sm := &StateManager{
		filePath:     path,
		enabled:      enabled,
		userExplicit: explicit,
		logger:       logger,
		data: AppState{
			Regions: make(map[string]*RegionState),
		},
	}

	if sm.enabled && sm.filePath != "" {
		if err := sm.Load(); err != nil {
			if logger != nil {
				logger.LogWarn("State load failed (%v). Starting with fresh state.", err)
			}
		}
	}

	return sm
}

func (sm *StateManager) IsEnabled() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.enabled
}

func (sm *StateManager) GetRegion(region string) *RegionState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.data.Regions == nil {
		return nil
	}
	st, exists := sm.data.Regions[region]
	if !exists || st == nil {
		return nil
	}
	// Return clone
	cp := *st
	if st.LastModified != nil {
		lm := *st.LastModified
		cp.LastModified = &lm
	}
	return &cp
}

func (sm *StateManager) UpdateRegion(region string, st *RegionState) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.data.Regions == nil {
		sm.data.Regions = make(map[string]*RegionState)
	}
	sm.data.Regions[region] = st
}

func (sm *StateManager) GetLastWorkflowDispatch() *time.Time {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.data.LastWorkflowDispatch == nil {
		return nil
	}
	t := *sm.data.LastWorkflowDispatch
	return &t
}

func (sm *StateManager) SetLastWorkflowDispatch(t time.Time) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	utc := t.UTC()
	sm.data.LastWorkflowDispatch = &utc
}

func (sm *StateManager) Load() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.enabled || sm.filePath == "" {
		return nil
	}

	data, err := os.ReadFile(sm.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var state AppState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("invalid state JSON: %w", err)
	}

	if state.Regions == nil {
		state.Regions = make(map[string]*RegionState)
	}
	sm.data = state
	return nil
}

func (sm *StateManager) Save() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !sm.enabled || sm.filePath == "" {
		return nil
	}

	dir := filepath.Dir(sm.filePath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return sm.handleSaveError(fmt.Errorf("failed to create directory %s: %w", dir, err))
		}
	}

	bytes, err := json.MarshalIndent(sm.data, "", "  ")
	if err != nil {
		return sm.handleSaveError(fmt.Errorf("failed to marshal state: %w", err))
	}

	tmpPath := sm.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, bytes, 0644); err != nil {
		return sm.handleSaveError(fmt.Errorf("failed to write state file %s: %w", tmpPath, err))
	}

	if err := os.Rename(tmpPath, sm.filePath); err != nil {
		// Fallback for cross-device renames or Windows replace locks
		if removeErr := os.Remove(sm.filePath); removeErr == nil || os.IsNotExist(removeErr) {
			if err2 := os.WriteFile(sm.filePath, bytes, 0644); err2 == nil {
				os.Remove(tmpPath)
				return nil
			}
		}
		return sm.handleSaveError(fmt.Errorf("failed to atomic replace state file %s: %w", sm.filePath, err))
	}

	return nil
}

func (sm *StateManager) handleSaveError(err error) error {
	if sm.userExplicit {
		// User explicitly asked for state persistence -> Fatal error
		return fmt.Errorf("FATAL state save error: %w", err)
	}

	// Auto-enabled mode (e.g. Linux default) -> Soft-disable for remainder of run
	sm.enabled = false
	if sm.logger != nil {
		sm.logger.LogWarn("State persistence write failed (%v). Soft-disabling state saving for remainder of run.", err)
	}
	return nil
}

// ParseDateTime attempts to robustly parse various date string formats.
func ParseDateTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date string")
	}

	// Check if string is a numeric Unix timestamp
	if unixSec, err := strconv.ParseInt(s, 10, 64); err == nil {
		if unixSec > 1e11 {
			// Milliseconds
			return time.UnixMilli(unixSec).UTC(), nil
		}
		return time.Unix(unixSec, 0).UTC(), nil
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		time.RFC850,
		time.ANSIC,
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
		"02 Jan 2006 15:04:05 GMT",
		"Mon, 02 Jan 2006 15:04:05 GMT",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date %q", s)
}
