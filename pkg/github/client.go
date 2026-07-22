package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/crypto"
	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/output"
	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/state"
)

type Config struct {
	Token                   string
	CredentialEncryptionKey string
	Repo                    string // e.g. "maxisoft-gaming/WowAHaha"
	WorkflowID              string // e.g. "run_every_hour.yml"
	Ref                     string // e.g. "main"
	DebounceInterval        time.Duration
	BaseURL                 string
	HTTPTimeout             time.Duration
	Disabled                bool
}

type WorkflowRun struct {
	ID         int64     `json:"id"`
	Status     string    `json:"status"` // "in_progress", "queued", "completed"
	Conclusion string    `json:"conclusion"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type workflowRunsResponse struct {
	TotalCount   int           `json:"total_count"`
	WorkflowRuns []WorkflowRun `json:"workflow_runs"`
}

type Client struct {
	cfg        Config
	token      string
	httpClient *http.Client
	logger     *output.OutputHandler
}

func NewClient(cfg Config, logger *output.OutputHandler) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.github.com"
	}
	if cfg.WorkflowID == "" {
		cfg.WorkflowID = "run_every_hour.yml"
	}
	if cfg.Ref == "" {
		cfg.Ref = "main"
	}
	if cfg.DebounceInterval <= 0 {
		cfg.DebounceInterval = 2 * time.Minute
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 15 * time.Second
	}

	token := crypto.DecryptIfNeeded(cfg.Token, cfg.CredentialEncryptionKey)

	return &Client{
		cfg:        cfg,
		token:      token,
		httpClient: &http.Client{Timeout: cfg.HTTPTimeout},
		logger:     logger,
	}
}

func (c *Client) IsDisabled() bool {
	return c.cfg.Disabled || c.token == "" || c.cfg.Repo == ""
}

// GetActiveWorkflowRuns checks if there are any running or queued workflows for the repository.
func (c *Client) GetActiveWorkflowRuns(ctx context.Context) ([]WorkflowRun, error) {
	if c.IsDisabled() {
		return nil, nil
	}

	u := fmt.Sprintf("%s/repos/%s/actions/runs?per_page=20", c.cfg.BaseURL, c.cfg.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch workflow runs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	var res workflowRunsResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode workflow runs: %w", err)
	}

	var active []WorkflowRun
	for _, run := range res.WorkflowRuns {
		if run.Status == "in_progress" || run.Status == "queued" || run.Status == "requested" || run.Status == "waiting" {
			active = append(active, run)
		}
	}

	return active, nil
}

// GetLatestCompletedRun returns the most recent completed workflow run timestamp.
func (c *Client) GetLatestCompletedRun(ctx context.Context) (*time.Time, error) {
	if c.IsDisabled() {
		return nil, nil
	}

	u := fmt.Sprintf("%s/repos/%s/actions/runs?status=completed&per_page=5", c.cfg.BaseURL, c.cfg.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch completed runs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var res workflowRunsResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	if len(res.WorkflowRuns) == 0 {
		return nil, nil
	}

	latest := res.WorkflowRuns[0].UpdatedAt
	if latest.IsZero() {
		latest = res.WorkflowRuns[0].CreatedAt
	}
	return &latest, nil
}

// DispatchWorkflow triggers a workflow_dispatch event on GitHub Actions.
func (c *Client) DispatchWorkflow(ctx context.Context, inputs map[string]interface{}) error {
	if c.IsDisabled() {
		return fmt.Errorf("GitHub dispatch is disabled or unconfigured")
	}

	u := fmt.Sprintf("%s/repos/%s/actions/workflows/%s/dispatches", c.cfg.BaseURL, c.cfg.Repo, c.cfg.WorkflowID)

	payload := map[string]interface{}{
		"ref": c.cfg.Ref,
	}
	if len(inputs) > 0 {
		payload["inputs"] = inputs
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal dispatch payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create dispatch request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("dispatch request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub dispatch returned status %d: %s", resp.StatusCode, string(body))
	}

	if c.logger != nil {
		c.logger.LogInfo("Successfully dispatched GitHub Actions workflow %s on repo %s (ref: %s)", c.cfg.WorkflowID, c.cfg.Repo, c.cfg.Ref)
	}

	return nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "WowAHaha-Bnet-Change-Detector")
}

// EvaluateTrigger determines whether a new workflow run should be dispatched based on debounce and running workflow checks.
func (c *Client) EvaluateTrigger(ctx context.Context, latestAuctionLM *time.Time, lastDispatch *time.Time, sm *state.StateManager) (bool, string) {
	if c.IsDisabled() {
		return false, "github_disabled"
	}

	now := time.Now().UTC()

	// Check 1: 2-minute Debounce
	if lastDispatch != nil && !lastDispatch.IsZero() {
		if now.Sub(*lastDispatch) < c.cfg.DebounceInterval {
			remaining := c.cfg.DebounceInterval - now.Sub(*lastDispatch)
			return false, fmt.Sprintf("debounced_wait_%.0fs", remaining.Seconds())
		}
	}

	// Check 2: Active running/queued workflows on GitHub
	activeRuns, err := c.GetActiveWorkflowRuns(ctx)
	if err != nil {
		if c.logger != nil {
			c.logger.LogWarn("Could not check active GitHub workflow runs: %v", err)
		}
	} else if len(activeRuns) > 0 {
		return false, fmt.Sprintf("active_workflow_running(count=%d)", len(activeRuns))
	}

	// Check 3: Compare latest auction timestamp against latest completed workflow run
	if latestAuctionLM != nil && !latestAuctionLM.IsZero() {
		latestRunTime, err := c.GetLatestCompletedRun(ctx)
		if err == nil && latestRunTime != nil && !latestRunTime.IsZero() {
			if !latestAuctionLM.After(*latestRunTime) {
				return false, fmt.Sprintf("workflow_already_up_to_date(run_at=%s, bnet_at=%s)",
					latestRunTime.UTC().Format(time.RFC3339),
					latestAuctionLM.UTC().Format(time.RFC3339))
			}
		}
	}

	return true, "bnet_data_updated"
}
