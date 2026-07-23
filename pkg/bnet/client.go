package bnet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/crypto"
	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/output"
	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/state"
)

type Config struct {
	ClientID                string
	ClientSecret            string
	CredentialEncryptionKey string
	OAuthTokenURI           string
	Regions                 []string
	Locale                  string
	MaxRetries              int
	HTTPTimeout             time.Duration
}

type HeadCheckResult struct {
	Region       string
	URL          string
	StatusCode   int
	LastModified *time.Time
	ETag         string
	ContentLen   int64
	Date         *time.Time
	Header       http.Header
}

type Client struct {
	cfg          Config
	httpClient   *http.Client
	logger       *output.OutputHandler
	clientID     string
	clientSecret string

	tokenMu     sync.RWMutex
	accessToken string
	tokenExpiry time.Time
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

func NewClient(cfg Config, logger *output.OutputHandler) *Client {
	if cfg.OAuthTokenURI == "" {
		cfg.OAuthTokenURI = "https://oauth.battle.net/token"
	}
	if len(cfg.Regions) == 0 {
		cfg.Regions = []string{"eu", "us", "kr", "tw"}
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 15 * time.Second
	}

	clientID := crypto.DecryptIfNeeded(cfg.ClientID, cfg.CredentialEncryptionKey)
	clientSecret := crypto.DecryptIfNeeded(cfg.ClientSecret, cfg.CredentialEncryptionKey)

	return &Client{
		cfg:          cfg,
		httpClient:   &http.Client{Timeout: cfg.HTTPTimeout},
		logger:       logger,
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

func (c *Client) GetAccessToken(ctx context.Context) (string, error) {
	c.tokenMu.RLock()
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		token := c.accessToken
		c.tokenMu.RUnlock()
		return token, nil
	}
	c.tokenMu.RUnlock()

	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	// Double check
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.OAuthTokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.clientID, c.clientSecret)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tok tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	c.accessToken = tok.AccessToken
	// Subtract 60 seconds buffer for clock skew / safety
	expirySec := tok.ExpiresIn - 60
	if expirySec <= 0 {
		expirySec = 300
	}
	c.tokenExpiry = time.Now().Add(time.Duration(expirySec) * time.Second)

	return c.accessToken, nil
}

// RegionHost returns the base API host for a given region.
func RegionHost(region string) string {
	r := strings.ToLower(strings.TrimSpace(region))
	switch r {
	case "cn":
		return "https://gateway.battlenet.com.cn"
	default:
		return fmt.Sprintf("https://%s.api.blizzard.com", r)
	}
}

// BuildCommoditiesURL constructs the commodities auction endpoint for a region.
func BuildCommoditiesURL(region, locale string) string {
	r := strings.ToLower(strings.TrimSpace(region))
	host := RegionHost(r)
	ns := fmt.Sprintf("dynamic-%s", r)

	u := fmt.Sprintf("%s/data/wow/auctions/commodities?namespace=%s", host, ns)
	if locale != "" && locale != "auto" {
		u += fmt.Sprintf("&locale=%s", locale)
	}
	return u
}

// CheckCommoditiesHead sends an HTTP HEAD request for a region's commodities auctions.
func (c *Client) CheckCommoditiesHead(ctx context.Context, region string) (*HeadCheckResult, error) {
	token, err := c.GetAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain token: %w", err)
	}

	reqURL := BuildCommoditiesURL(region, c.cfg.Locale)
	// Use GET instead of HEAD. BNet API CDNs heavily cache or break HEAD requests.
	// We will close the response body immediately after reading headers.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GET request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("GET request failed for region %s: %w", region, err)
	}
	defer resp.Body.Close()

	res := &HeadCheckResult{
		Region:     region,
		URL:        reqURL,
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		ETag:       resp.Header.Get("ETag"),
	}

	if clStr := resp.Header.Get("Content-Length"); clStr != "" {
		if cl, err := strconv.ParseInt(clStr, 10, 64); err == nil {
			res.ContentLen = cl
		}
	}

	if lmStr := resp.Header.Get("Last-Modified"); lmStr != "" {
		if t, err := state.ParseDateTime(lmStr); err == nil {
			res.LastModified = &t
		}
	}

	if dateStr := resp.Header.Get("Date"); dateStr != "" {
		if t, err := state.ParseDateTime(dateStr); err == nil {
			res.Date = &t
		}
	}

	return res, nil
}

func (c *Client) doWithRetry(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<attempt)*200*time.Millisecond + time.Duration(rand.Intn(100))*time.Millisecond
			if c.logger != nil {
				c.logger.LogDebug("Retrying HTTP request to %s (attempt %d/%d) after %v...", req.URL.String(), attempt, c.cfg.MaxRetries, backoff)
			}
			time.Sleep(backoff)
		}

		// Re-clone request body if any (for POST requests)
		var reqClone *http.Request
		if req.Body != nil {
			reqClone = req.Clone(req.Context())
		} else {
			reqClone = req
		}

		resp, err = c.httpClient.Do(reqClone)
		if err == nil {
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotModified {
				return resp, nil
			}
			// Transient errors
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				resp.Body.Close()
				continue
			}
			// Permanent client error (e.g. 401, 403, 404)
			return resp, nil
		}
	}

	return resp, err
}
