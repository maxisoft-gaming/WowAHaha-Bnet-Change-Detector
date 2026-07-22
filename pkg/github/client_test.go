package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/output"
)

func TestGitHubClient_WorkflowRunsAndDispatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test_github_token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/actions/runs"):
			w.Header().Set("Content-Type", "application/json")
			if strings.Contains(r.URL.RawQuery, "status=completed") {
				fmt.Fprintln(w, `{"total_count": 1, "workflow_runs": [{"id": 101, "status": "completed", "updated_at": "2026-07-22T17:00:00Z"}]}`)
			} else {
				fmt.Fprintln(w, `{"total_count": 0, "workflow_runs": []}`)
			}

		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/dispatches"):
			w.WriteHeader(http.StatusNoContent)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	logger := output.NewOutputHandler(output.FormatText, true)
	client := NewClient(Config{
		Token:            "test_github_token",
		Repo:             "myorg/myrepo",
		WorkflowID:       "run_every_hour.yml",
		BaseURL:          server.URL,
		DebounceInterval: 2 * time.Minute,
	}, logger)

	ctx := context.Background()

	// Test GetActiveWorkflowRuns -> should be 0 active
	active, err := client.GetActiveWorkflowRuns(ctx)
	if err != nil {
		t.Fatalf("GetActiveWorkflowRuns failed: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("Expected 0 active workflow runs, got %d", len(active))
	}

	// Test EvaluateTrigger -> bnet date (18:00) is NEWER than latest run (17:00)
	bnetDate := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)
	shouldTrigger, reason := client.EvaluateTrigger(ctx, &bnetDate, nil, nil)
	if !shouldTrigger {
		t.Errorf("Expected trigger to be true, got false (reason: %s)", reason)
	}

	// Test DispatchWorkflow
	if err := client.DispatchWorkflow(ctx, nil); err != nil {
		t.Fatalf("DispatchWorkflow failed: %v", err)
	}
}

func TestGitHubClient_DebounceEnforcement(t *testing.T) {
	logger := output.NewOutputHandler(output.FormatText, true)
	client := NewClient(Config{
		Token:            "test_github_token",
		Repo:             "myorg/myrepo",
		DebounceInterval: 2 * time.Minute,
	}, logger)

	ctx := context.Background()
	recentDispatch := time.Now().UTC().Add(-30 * time.Second) // 30 seconds ago
	bnetDate := time.Now().UTC()

	shouldTrigger, reason := client.EvaluateTrigger(ctx, &bnetDate, &recentDispatch, nil)
	if shouldTrigger {
		t.Fatalf("Expected debounce to block trigger, but got shouldTrigger=true")
	}

	if !strings.HasPrefix(reason, "debounced_wait_") {
		t.Errorf("Expected debounced reason prefix, got: %s", reason)
	}
}
