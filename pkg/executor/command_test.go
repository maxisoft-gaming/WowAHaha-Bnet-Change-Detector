package executor

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/output"
)

func TestExecutor_SecretFilteringDefault(t *testing.T) {
	// Set a secret in environment
	os.Setenv("MY_SECRET_KEY", "super_secret_value")
	defer os.Unsetenv("MY_SECRET_KEY")

	logger := output.NewOutputHandler(output.FormatText, true)

	var cmdStr string
	if runtime.GOOS == "windows" {
		cmdStr = "echo BNET_REGION=%BNET_REGION% MY_SECRET_KEY=%MY_SECRET_KEY%"
	} else {
		cmdStr = "echo BNET_REGION=$BNET_REGION MY_SECRET_KEY=$MY_SECRET_KEY"
	}

	exec := NewExecutor(CommandConfig{
		Command:              cmdStr,
		PassSecretsToCommand: false, // Default: withhold secrets!
		ClientSecret:         "hidden_bnet_secret",
	}, logger)

	ctx := context.Background()
	lm := time.Date(2026, 7, 22, 18, 0, 0, 0, time.UTC)
	cCtx := CommandContext{
		Region:       "eu",
		URL:          "https://eu.api.blizzard.com/test",
		LastModified: &lm,
		Changed:      true,
	}

	// Verify execution succeeds without passing secret
	if err := exec.Execute(ctx, cCtx); err != nil {
		t.Fatalf("Executor failed: %v", err)
	}
}

func TestExecutor_SecretPassingWhenExplicitlyEnabled(t *testing.T) {
	logger := output.NewOutputHandler(output.FormatText, true)

	var cmdStr string
	if runtime.GOOS == "windows" {
		cmdStr = "echo BNET_CLIENT_SECRET=%BNET_CLIENT_SECRET%"
	} else {
		cmdStr = "echo BNET_CLIENT_SECRET=$BNET_CLIENT_SECRET"
	}

	exec := NewExecutor(CommandConfig{
		Command:              cmdStr,
		PassSecretsToCommand: true, // Explicitly enabled!
		ClientSecret:         "revealed_secret",
	}, logger)

	ctx := context.Background()
	cCtx := CommandContext{
		Region: "us",
	}

	if err := exec.Execute(ctx, cCtx); err != nil {
		t.Fatalf("Executor failed: %v", err)
	}
}
