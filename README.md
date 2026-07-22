# WowAHaha Battle.net Change Detector

`bnet-change-detector` is a lightweight, open-source (GPL-3.0), high-performance monitoring daemon & CLI tool written in **Go**. It periodically checks World of Warcraft Auction House data across multiple regions via the Battle.net Game Data API (using HTTP `HEAD` requests) and triggers downstream actions—such as a GitHub Actions `workflow_dispatch` event or executing an external local command—with strict rate-limiting, anti-saturation debounce protections, OS-aware state persistence, and minimal resource usage (~5-10MB RAM, <5ms startup time).

---

## Key Features

- **HTTP HEAD Monitoring**: Checks `Last-Modified`, `ETag`, `Date`, and `Content-Length` headers across multiple regions (`eu`, `us`, `kr`, `tw`) without pulling large payload bodies.
- **GitHub Actions Integration**: Triggers `workflow_dispatch` events on GitHub repositories when auction data updates.
- **Debounce & Anti-Saturation Protection**: Enforces a 2-minute debounce delay between dispatches and inspects active/running workflows on GitHub before triggering new runs.
- **External Command Execution**: Spawns custom shell commands (e.g. running a local C# application or container execution) upon change detection.
- **Subprocess Environment Security**: Secrets (`clientId`, `clientSecret`, `githubToken`) are withheld from subprocess environments by default unless `--pass-secrets-to-command` is explicitly set.
- **OS-Aware State Persistence**:
  - **Linux**: Enabled by default targeting `/var/run/bnet-change-detector-state.json`. If IO/permission error occurs, logs a warning and soft-disables state persistence for the remainder of the run.
  - **Windows**: Disabled by default.
  - **Explicit User Override**: If `--enable-state` or `--state-file` is explicitly specified by the end-user, any save error becomes a **fatal error**.
- **1-Line Tick Output**: Diagnostics and logs go strictly to `stderr`, while `stdout` emits exactly **one line per loop tick** (Text or JSON `--output-format json`), making stdout pipeable to `jq` or log collectors.
- **Compatible with DumbDumbEncryption**: Directly decrypts `!enc:<base64>` credential strings using the C# `DumbDumbEncryption` key.
- **Deployment Modes**: Loop mode (daemon), Single-Shot mode (cron / systemd timer), Docker, Podman, Systemd service/timer.

---

## Installation & Building

### From Source
```bash
git clone https://github.com/maxisoft-gaming/WowAHaha-Bnet-Change-Detector.git
cd WowAHaha-Bnet-Change-Detector
make build
```

### Cross-Compilation
```bash
make cross-compile
```
Outputs static binaries into `bin/` for Windows (x64, ARM64) and Linux (x64, ARM64, ARMv7).

---

## Quick Usage

### Command Line Options

```
Usage of bnet-change-detector:
  -mode string
        Execution mode: 'single' (cron/timer) or 'loop' (daemon) (default "single")
  -interval duration
        Check interval for loop mode (default 1m0s)
  -output-format string
        Output format: 'text' or 'json' (default "text")
  -verbose
        Enable verbose debug logs to stderr
  -client-id string
        Battle.net API Client ID (supports !enc:)
  -client-secret string
        Battle.net API Client Secret (supports !enc:)
  -credential-encryption-key string
        Encryption key for !enc: strings
  -regions string
        Comma-separated list of regions to monitor (default "eu,us,kr,tw")
  -previous-snapshot-date string
        Override previous snapshot date (ISO8601, RFC3339, RFC2822, Unix)
  -github-token string
        GitHub Personal Access Token (supports !enc:)
  -github-repo string
        GitHub repository (e.g. maxisoft-gaming/WowAHaha)
  -github-workflow-id string
        GitHub workflow filename or ID (default "run_every_hour.yml")
  -no-github-dispatch
        Disable GitHub Actions workflow dispatch
  -command string
        External command to run when an update is detected
  -pass-secrets-to-command
        Pass secrets in environment to external command
  -state-file string
        Path to state JSON file
  -enable-state
        Explicitly enable state persistence
  -disable-state
        Explicitly disable state persistence
```

### Environment Variables

All parameters can also be configured via environment variables:
- `BNET_CLIENT_ID` or `AHaha_BattleNetWebApi:clientId`
- `BNET_CLIENT_SECRET` or `AHaha_BattleNetWebApi:clientSecret`
- `BNET_CREDENTIAL_ENCRYPTION_KEY` or `AHaha_BattleNetWebApi:CredentialEncryptionKey`
- `GITHUB_TOKEN`
- `GITHUB_REPO`
- `DETECTOR_MODE`
- `OUTPUT_FORMAT`

---

## Deployment Examples

### Single-Shot (Cron / Systemd Timer)
```bash
bnet-change-detector \
  --mode=single \
  --client-id="!enc:..." \
  --client-secret="!enc:..." \
  --credential-encryption-key="TMP_CHANGE_ME" \
  --github-token="ghp_..." \
  --github-repo="maxisoft-gaming/WowAHaha"
```
Exit codes in single-shot mode:
- `0`: No change detected / success.
- `10`: Change detected and action triggered.
- `1`: Error encountered.

### Daemon (Loop Mode)
```bash
bnet-change-detector \
  --mode=loop \
  --interval=60s \
  --output-format=json
```

### Docker / Podman
```bash
docker run -d \
  --name bnet-detector \
  -e BNET_CLIENT_ID="f5f7235c81384695bef666871e5ca18d" \
  -e BNET_CLIENT_SECRET="!enc:ekM9JgdReFkWXAUGC0lrBx8yeUdDBBwMRA2bB1gcGDI=" \
  -e BNET_CREDENTIAL_ENCRYPTION_KEY="TMP_CHANGE_ME" \
  -e GITHUB_TOKEN="ghp_..." \
  -e GITHUB_REPO="maxisoft-gaming/WowAHaha" \
  ghcr.io/maxisoft-gaming/wowahaha-bnet-change-detector:latest
```

---

## License

This project is licensed under the [GNU General Public License v3.0 (GPL-3.0)](LICENSE).
