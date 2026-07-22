package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/bnet"
	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/crypto"
	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/executor"
	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/github"
	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/output"
	"github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector/pkg/state"
)

var version = "dev"

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		v := strings.ToLower(val)
		return v == "true" || v == "1" || v == "yes" || v == "on"
	}
	return defaultVal
}

func main() {
	var (
		modeFlag               = flag.String("mode", getEnvOrDefault("DETECTOR_MODE", "single"), "Execution mode: 'single' (cron/timer) or 'loop' (daemon)")
		intervalFlag           = flag.Duration("interval", 60*time.Second, "Check interval for loop mode (e.g. 30s, 60s, 5m)")
		outputFormatFlag       = flag.String("output-format", getEnvOrDefault("OUTPUT_FORMAT", "text"), "Output format: 'text' or 'json'")
		verboseFlag            = flag.Bool("verbose", getEnvBool("VERBOSE", false), "Enable verbose debug logs to stderr")
		versionFlag            = flag.Bool("version", false, "Print version and exit")

		// BNet Flags
		clientIDFlag           = flag.String("client-id", getEnvOrDefault("BNET_CLIENT_ID", getEnvOrDefault("AHaha_BattleNetWebApi:clientId", "")), "Battle.net API Client ID (supports !enc:)")
		clientSecretFlag       = flag.String("client-secret", getEnvOrDefault("BNET_CLIENT_SECRET", getEnvOrDefault("AHaha_BattleNetWebApi:clientSecret", "")), "Battle.net API Client Secret (supports !enc:)")
		credEncryptionKeyFlag  = flag.String("credential-encryption-key", getEnvOrDefault("BNET_CREDENTIAL_ENCRYPTION_KEY", getEnvOrDefault("AHaha_BattleNetWebApi:CredentialEncryptionKey", "")), "Encryption key for !enc: strings")
		oauthTokenURIFlag      = flag.String("oauth-token-uri", getEnvOrDefault("BNET_OAUTH_TOKEN_URI", "https://oauth.battle.net/token"), "Battle.net OAuth Token URI")
		regionsFlag            = flag.String("regions", getEnvOrDefault("BNET_REGIONS", "eu,us,kr,tw"), "Comma-separated list of regions to monitor")
		localeFlag             = flag.String("locale", getEnvOrDefault("BNET_LOCALE", "auto"), "Battle.net API locale (e.g. auto, en_US)")
		prevSnapshotDateFlag   = flag.String("previous-snapshot-date", getEnvOrDefault("BNET_PREVIOUS_SNAPSHOT_DATE", ""), "Override previous snapshot date (ISO8601, RFC3339, RFC2822, Unix)")

		// GitHub Flags
		githubTokenFlag        = flag.String("github-token", getEnvOrDefault("GITHUB_TOKEN", ""), "GitHub Personal Access Token (supports !enc:)")
		githubRepoFlag         = flag.String("github-repo", getEnvOrDefault("GITHUB_REPO", ""), "GitHub repository (e.g. maxisoft-gaming/WowAHaha)")
		githubWorkflowIDFlag   = flag.String("github-workflow-id", getEnvOrDefault("GITHUB_WORKFLOW_ID", "run_every_hour.yml"), "GitHub workflow filename or ID")
		githubRefFlag          = flag.String("github-ref", getEnvOrDefault("GITHUB_REF", "main"), "GitHub branch/ref for workflow dispatch")
		workflowInputsFlag     = flag.String("workflow-inputs", getEnvOrDefault("GITHUB_WORKFLOW_INPUTS", ""), "Optional workflow dispatch inputs (key=value,key2=value2)")
		noGithubDispatchFlag   = flag.Bool("no-github-dispatch", getEnvBool("DISABLE_GITHUB_DISPATCH", false), "Disable GitHub Actions workflow dispatch")
		debounceIntervalFlag   = flag.Duration("debounce-interval", 2*time.Minute, "Debounce interval between workflow dispatches (e.g. 2m)")

		// Subprocess Command Flags
		commandFlag            = flag.String("command", getEnvOrDefault("ON_UPDATE_COMMAND", ""), "External command to run when an update is detected")
		passSecretsFlag        = flag.Bool("pass-secrets-to-command", getEnvBool("PASS_SECRETS_TO_COMMAND", false), "Pass secrets in environment to external command")

		// State Flags
		stateFileFlag          = flag.String("state-file", getEnvOrDefault("STATE_FILE", ""), "Path to state JSON file")
		enableStateFlag        = flag.Bool("enable-state", false, "Explicitly enable state persistence")
		disableStateFlag       = flag.Bool("disable-state", false, "Explicitly disable state persistence")
	)

	flag.Parse()

	if *versionFlag {
		fmt.Printf("bnet-change-detector version %s\n", version)
		os.Exit(0)
	}

	format := output.FormatText
	if strings.ToLower(*outputFormatFlag) == "json" {
		format = output.FormatJSON
	}

	logger := output.NewOutputHandler(format, *verboseFlag)

	// Process regions
	regionList := strings.Split(*regionsFlag, ",")
	cleanRegions := make([]string, 0, len(regionList))
	for _, r := range regionList {
		r = strings.TrimSpace(r)
		if r != "" {
			cleanRegions = append(cleanRegions, strings.ToLower(r))
		}
	}
	if len(cleanRegions) == 0 {
		cleanRegions = []string{"eu", "us", "kr", "tw"}
	}

	// Decrypt BNet credentials
	key := *credEncryptionKeyFlag
	clientID := crypto.DecryptIfNeeded(*clientIDFlag, key)
	clientSecret := crypto.DecryptIfNeeded(*clientSecretFlag, key)

	if clientID == "" || clientSecret == "" {
		logger.LogError("Battle.net ClientID and ClientSecret are required! Set via --client-id / --client-secret or env variables.")
		os.Exit(1)
	}

	// Process state options
	var enableState *bool
	if *enableStateFlag {
		t := true
		enableState = &t
	} else if *disableStateFlag {
		f := false
		enableState = &f
	}

	stateMgr := state.NewStateManager(*stateFileFlag, enableState, logger)

	// Process CLI override date if specified
	var cliPrevDate *time.Time
	if *prevSnapshotDateFlag != "" {
		t, err := state.ParseDateTime(*prevSnapshotDateFlag)
		if err != nil {
			logger.LogError("Invalid --previous-snapshot-date: %v", err)
			os.Exit(1)
		}
		cliPrevDate = &t
		logger.LogInfo("Overriding previous snapshot date with CLI input: %s", t.Format(time.RFC3339))
	}

	// Initialize BNet Client
	bnetClient := bnet.NewClient(bnet.Config{
		ClientID:                clientID,
		ClientSecret:            clientSecret,
		CredentialEncryptionKey: key,
		OAuthTokenURI:           *oauthTokenURIFlag,
		Regions:                 cleanRegions,
		Locale:                  *localeFlag,
	}, logger)

	// Initialize GitHub Client
	githubClient := github.NewClient(github.Config{
		Token:                   *githubTokenFlag,
		CredentialEncryptionKey: key,
		Repo:                    *githubRepoFlag,
		WorkflowID:              *githubWorkflowIDFlag,
		Ref:                     *githubRefFlag,
		DebounceInterval:        *debounceIntervalFlag,
		Disabled:                *noGithubDispatchFlag,
	}, logger)

	// Initialize Command Executor
	cmdExec := executor.NewExecutor(executor.CommandConfig{
		Command:              *commandFlag,
		PassSecretsToCommand: *passSecretsFlag,
		ClientID:             clientID,
		ClientSecret:         clientSecret,
		GitHubToken:          *githubTokenFlag,
	}, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runTick := func() (bool, error) {
		tickTime := time.Now().UTC()
		tickRes := &output.TickResult{
			Timestamp: tickTime,
			Regions:   make(map[string]*output.RegionTick),
		}

		var latestAuctionLM *time.Time
		var updatedRegion string
		anyChanged := false

		for _, region := range cleanRegions {
			hRes, err := bnetClient.CheckCommoditiesHead(ctx, region)
			if err != nil {
				logger.LogError("Failed HEAD check for region %s: %v", region, err)
				tickRes.Error = fmt.Sprintf("bnet_head_error(%s): %v", region, err)
				continue
			}

			prevSt := stateMgr.GetRegion(region)
			rChanged := false

			if prevSt == nil {
				// No previous state stored
				if cliPrevDate != nil && hRes.LastModified != nil {
					rChanged = hRes.LastModified.After(*cliPrevDate)
				} else {
					rChanged = true
				}
			} else {
				if hRes.LastModified != nil && prevSt.LastModified != nil {
					rChanged = hRes.LastModified.After(*prevSt.LastModified)
				} else if hRes.ETag != "" && prevSt.ETag != "" {
					rChanged = hRes.ETag != prevSt.ETag
				} else if hRes.ContentLen > 0 && prevSt.ContentLen > 0 {
					rChanged = hRes.ContentLen != prevSt.ContentLen
				} else {
					rChanged = true
				}
			}

			rTick := &output.RegionTick{
				LastModified: hRes.LastModified,
				ETag:         hRes.ETag,
				ContentLen:   hRes.ContentLen,
				Changed:      rChanged,
			}
			tickRes.Regions[region] = rTick

			if rChanged {
				anyChanged = true
				updatedRegion = region
				if hRes.LastModified != nil {
					if latestAuctionLM == nil || hRes.LastModified.After(*latestAuctionLM) {
						latestAuctionLM = hRes.LastModified
					}
				}
			}

			// Update state in memory
			stateMgr.UpdateRegion(region, &state.RegionState{
				LastModified: hRes.LastModified,
				ETag:         hRes.ETag,
				ContentLen:   hRes.ContentLen,
				LastChecked:  tickTime,
			})
		}

		tickRes.Changed = anyChanged

		actionTriggered := false
		triggerReason := "none"

		if anyChanged || cliPrevDate != nil {
			lastDispatch := stateMgr.GetLastWorkflowDispatch()

			// Evaluate GitHub Dispatch
			if !githubClient.IsDisabled() {
				shouldDispatch, reason := githubClient.EvaluateTrigger(ctx, latestAuctionLM, lastDispatch, stateMgr)
				triggerReason = reason

				if shouldDispatch {
					inputs := parseWorkflowInputs(*workflowInputsFlag)

					if err := githubClient.DispatchWorkflow(ctx, inputs); err != nil {
						logger.LogError("Failed to dispatch GitHub workflow: %v", err)
					} else {
						actionTriggered = true
						stateMgr.SetLastWorkflowDispatch(tickTime)
					}
				}
			}

			// External Command Execution
			if *commandFlag != "" {
				cmdCtx := executor.CommandContext{
					Region:       updatedRegion,
					LastModified: latestAuctionLM,
					Changed:      anyChanged,
					Timestamp:    tickTime,
				}
				if err := cmdExec.Execute(ctx, cmdCtx); err != nil {
					logger.LogError("Failed to execute command: %v", err)
				} else {
					actionTriggered = true
					if triggerReason == "none" {
						triggerReason = "command_executed"
					}
				}
			}
		}

		tickRes.ActionTriggered = actionTriggered
		tickRes.TriggerReason = triggerReason

		// Save state if enabled
		if err := stateMgr.Save(); err != nil {
			logger.LogError("State save failed: %v", err)
			// If error is returned from Save(), it means explicit mode fatal error
			return false, err
		}

		// Write single-line tick to stdout
		if err := logger.WriteTick(tickRes); err != nil {
			logger.LogError("Failed writing tick to stdout: %v", err)
		}

		return anyChanged, nil
	}

	mode := strings.ToLower(*modeFlag)
	if mode == "loop" || mode == "daemon" {
		logger.LogInfo("Starting WowAHaha Battle.net Change Detector in LOOP mode (interval: %v)...", *intervalFlag)
		ticker := time.NewTicker(*intervalFlag)
		defer ticker.Stop()

		// Run immediate first tick
		if _, err := runTick(); err != nil {
			logger.LogError("Error in loop tick: %v", err)
		}

		for {
			select {
			case <-ctx.Done():
				logger.LogInfo("Received shutdown signal. Exiting gracefully.")
				os.Exit(0)
			case <-ticker.C:
				if _, err := runTick(); err != nil {
					logger.LogError("Error in loop tick: %v", err)
				}
			}
		}
	} else {
		// Single-shot mode (Cron / Timer)
		logger.LogInfo("Running WowAHaha Battle.net Change Detector in SINGLE-SHOT mode...")
		changed, err := runTick()
		if err != nil {
			logger.LogError("Single-shot execution failed: %v", err)
			os.Exit(1)
		}

		if changed {
			logger.LogInfo("Auction data change detected!")
			os.Exit(10) // Exit code 10 = change detected & handled
		}

		logger.LogInfo("No change detected.")
		os.Exit(0)
	}
}

func parseWorkflowInputs(raw string) map[string]interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	res := make(map[string]interface{})
	pairs := strings.Split(raw, ",")
	for _, p := range pairs {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			k := strings.TrimSpace(kv[0])
			v := strings.TrimSpace(kv[1])
			if k != "" {
				res[k] = v
			}
		}
	}
	if len(res) == 0 {
		return nil
	}
	return res
}
