package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/output"
)

type CommandConfig struct {
	Command              string
	PassSecretsToCommand bool
	ClientID             string
	ClientSecret         string
	GitHubToken          string
}

type CommandContext struct {
	Region       string
	URL          string
	LastModified *time.Time
	ETag         string
	ContentLen   int64
	Changed      bool
	Timestamp    time.Time
}

type Executor struct {
	cfg    CommandConfig
	logger *output.OutputHandler
}

func NewExecutor(cfg CommandConfig, logger *output.OutputHandler) *Executor {
	return &Executor{
		cfg:    cfg,
		logger: logger,
	}
}

func (e *Executor) Execute(ctx context.Context, cCtx CommandContext) error {
	cmdStr := strings.TrimSpace(e.cfg.Command)
	if cmdStr == "" {
		return nil
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/C", cmdStr)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", cmdStr)
	}

	// Filter environment variables
	envMap := make(map[string]string)
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) == 2 {
			k, v := pair[0], pair[1]
			// Withhold sensitive env vars by default unless explicitly allowed
			if !e.cfg.PassSecretsToCommand {
				lk := strings.ToLower(k)
				if strings.Contains(lk, "secret") || strings.Contains(lk, "token") || strings.Contains(lk, "password") {
					continue
				}
			}
			envMap[k] = v
		}
	}

	// Add BNet change context env vars
	if cCtx.Timestamp.IsZero() {
		cCtx.Timestamp = time.Now().UTC()
	}
	envMap["BNET_CHECK_TIMESTAMP"] = cCtx.Timestamp.UTC().Format(time.RFC3339)
	envMap["BNET_REGION"] = cCtx.Region
	envMap["BNET_URL"] = cCtx.URL
	envMap["BNET_ETAG"] = cCtx.ETag
	envMap["BNET_CONTENT_LENGTH"] = fmt.Sprintf("%d", cCtx.ContentLen)
	if cCtx.Changed {
		envMap["BNET_CHANGED"] = "true"
	} else {
		envMap["BNET_CHANGED"] = "false"
	}

	if cCtx.LastModified != nil {
		envMap["BNET_LAST_MODIFIED"] = cCtx.LastModified.UTC().Format(time.RFC3339)
	}

	if e.cfg.PassSecretsToCommand {
		if e.cfg.ClientID != "" {
			envMap["BNET_CLIENT_ID"] = e.cfg.ClientID
		}
		if e.cfg.ClientSecret != "" {
			envMap["BNET_CLIENT_SECRET"] = e.cfg.ClientSecret
		}
		if e.cfg.GitHubToken != "" {
			envMap["GITHUB_TOKEN"] = e.cfg.GitHubToken
		}
	}

	envSlice := make([]string, 0, len(envMap))
	for k, v := range envMap {
		envSlice = append(envSlice, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = envSlice

	if e.logger != nil {
		e.logger.LogInfo("Executing external command: %s (region: %s, pass_secrets: %v)", cmdStr, cCtx.Region, e.cfg.PassSecretsToCommand)
	}

	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		if e.logger != nil {
			e.logger.LogError("External command failed (%v). Output: %s", err, string(outputBytes))
		}
		return fmt.Errorf("external command exited with error: %w", err)
	}

	if e.logger != nil && len(outputBytes) > 0 {
		e.logger.LogDebug("External command output: %s", string(outputBytes))
	}

	return nil
}
