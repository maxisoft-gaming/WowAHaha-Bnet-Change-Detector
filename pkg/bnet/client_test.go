package bnet

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/output"
)

func TestClient_OAuthAndGetCheck(t *testing.T) {
	lmHeader := "Wed, 22 Jul 2026 18:00:00 GMT"
	expectedLM := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			user, pass, ok := r.BasicAuth()
			if !ok || user != "myclient" || pass != "mysecret" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"access_token": "mock_token_123", "token_type": "bearer", "expires_in": 3600}`)

		case "/data/wow/auctions/commodities":
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			auth := r.Header.Get("Authorization")
			if auth != "Bearer mock_token_123" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Last-Modified", lmHeader)
			w.Header().Set("ETag", `"etag_hash_999"`)
			w.Header().Set("Content-Length", "987654")
			w.WriteHeader(http.StatusOK)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	logger := output.NewOutputHandler(output.FormatText, true)
	client := NewClient(Config{
		ClientID:      "myclient",
		ClientSecret:  "mysecret",
		OAuthTokenURI: server.URL + "/token",
	}, logger)

	// Override URL creation for test by directly passing custom URL via custom head request or host
	ctx := context.Background()
	token, err := client.GetAccessToken(ctx)
	if err != nil {
		t.Fatalf("GetAccessToken failed: %v", err)
	}
	if token != "mock_token_123" {
		t.Errorf("Expected token mock_token_123, got: %s", token)
	}

	// Test GET request against server directly
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/data/wow/auctions/commodities", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.doWithRetry(req)
	if err != nil {
		t.Fatalf("HEAD request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("ETag") != `"etag_hash_999"` {
		t.Errorf("Unexpected ETag: %s", resp.Header.Get("ETag"))
	}

	lmParsed := resp.Header.Get("Last-Modified")
	if lmParsed != lmHeader {
		t.Errorf("Last-Modified mismatch: got %s, want %s", lmParsed, lmHeader)
	}
	_ = expectedLM
}
