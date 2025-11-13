# Phase 5.2: Webhook Notifications

## Overview

Phase 5.2 implements webhook notifications for proxmox-backup-go, allowing backup status reports to be sent to multiple platforms including Discord, Slack, Microsoft Teams, and generic webhook endpoints.

## Architecture

The webhook notification system follows the same architecture as Telegram and Email notifications:

1. **`WebhookNotifier`** - Implements the `Notifier` interface
2. **`NotificationAdapter`** - Converts `BackupStats` to `NotificationData`
3. **Orchestrator Integration** - Registered as a notification channel

### Key Components

- **`internal/notify/webhook.go`** (~350 lines) - Core webhook implementation with extensive debug logging
- **`internal/notify/webhook_payloads.go`** (~450 lines) - Payload formatters for Discord, Slack, Teams, Generic
- **`internal/config/config.go`** - Webhook configuration parsing from environment variables

## Configuration

### Environment Variables

```bash
# Enable webhook notifications
WEBHOOK_ENABLED=true

# Default format for all endpoints (generic, discord, slack, teams)
WEBHOOK_FORMAT=discord

# Connection settings
WEBHOOK_TIMEOUT=30          # seconds
WEBHOOK_MAX_RETRIES=3      # retry attempts
WEBHOOK_RETRY_DELAY=2      # seconds between retries

# Endpoint names (comma-separated)
WEBHOOK_ENDPOINTS=discord-alerts,slack-team,teams-channel
```

### Per-Endpoint Configuration

Each endpoint is configured with environment variables using the pattern:
`WEBHOOK_<NAME>_<PROPERTY>=<value>`

Example for Discord endpoint named "discord-alerts":

```bash
# Discord Configuration
WEBHOOK_DISCORD_ALERTS_NAME=discord-alerts
WEBHOOK_DISCORD_ALERTS_URL=https://discord.com/api/webhooks/123456/abcdefgh
WEBHOOK_DISCORD_ALERTS_FORMAT=discord
WEBHOOK_DISCORD_ALERTS_METHOD=POST
WEBHOOK_DISCORD_ALERTS_AUTH_TYPE=none
```

## Supported Platforms

### 1. Discord

Discord webhooks use embeds with color-coded status indicators and structured fields.

**Configuration:**
```bash
WEBHOOK_DISCORD_URL=https://discord.com/api/webhooks/YOUR_WEBHOOK_ID/YOUR_WEBHOOK_TOKEN
WEBHOOK_DISCORD_FORMAT=discord
```

**Payload Features:**
- Color-coded embeds (green=success, orange=warning, red=failure)
- Status emoji in title
- Structured fields for hostname, date, duration, size
- Storage status for local/secondary/cloud
- Issues summary with error/warning counts
- Top 5 log categories displayed

### 2. Slack

Slack webhooks use Block Kit format with sections and dividers.

**Configuration:**
```bash
WEBHOOK_SLACK_URL=https://hooks.slack.com/services/YOUR/WEBHOOK/PATH
WEBHOOK_SLACK_FORMAT=slack
```

**Payload Features:**
- Header block with status emoji
- Section blocks for status, backup details, storage
- Markdown formatting for bold/italic text
- Dividers for visual separation
- Context footer with script version

### 3. Microsoft Teams

Teams webhooks use Adaptive Cards v1.5 format.

**Configuration:**
```bash
WEBHOOK_TEAMS_URL=https://outlook.office.com/webhook/YOUR_WEBHOOK_URL
WEBHOOK_TEAMS_FORMAT=teams
```

**Payload Features:**
- Adaptive Card with FactSet
- Theme color based on status
- Structured facts for all backup metrics
- Text blocks for log categories
- Professional card layout

### 4. Generic JSON

Generic format for custom integrations and monitoring systems.

**Configuration:**
```bash
WEBHOOK_CUSTOM_URL=https://your-monitoring-system.com/webhook
WEBHOOK_CUSTOM_FORMAT=generic
```

**Payload Structure:**
```json
{
  "status": "success",
  "hostname": "pbs1.tis24.it",
  "timestamp": 1699999999,
  "timestamp_iso": "2025-11-11T12:00:00Z",
  "backup": {
    "file_name": "pbs1-backup-20251111.tar.xz",
    "size_bytes": 7558144,
    "size_human": "7.2 MiB",
    "duration_seconds": 120.5,
    "files_included": 1070,
    "files_missing": 0
  },
  "compression": {
    "type": "xz",
    "level": 9,
    "mode": "ultra",
    "ratio": 58.78
  },
  "storage": {
    "local": {
      "status": "ok",
      "count": 7,
      "free": "12.68 GB",
      "percent_num": 53.0
    },
    "secondary": { ... },
    "cloud": { ... }
  },
  "issues": {
    "errors": 0,
    "warnings": 0,
    "total": 0
  }
}
```

## Authentication Methods

### None (Default)
```bash
WEBHOOK_ENDPOINT1_AUTH_TYPE=none
```

### Bearer Token
```bash
WEBHOOK_ENDPOINT1_AUTH_TYPE=bearer
WEBHOOK_ENDPOINT1_AUTH_TOKEN=your-bearer-token-here
```

Sends: `Authorization: Bearer <token>`

### Basic Authentication
```bash
WEBHOOK_ENDPOINT1_AUTH_TYPE=basic
WEBHOOK_ENDPOINT1_AUTH_USER=username
WEBHOOK_ENDPOINT1_AUTH_PASS=password
```

Sends: `Authorization: Basic <base64(user:pass)>`

### HMAC-SHA256 Signature
```bash
WEBHOOK_ENDPOINT1_AUTH_TYPE=hmac
WEBHOOK_ENDPOINT1_AUTH_SECRET=your-secret-key
```

Sends:
- `X-Signature: <hmac-sha256-hex>`
- `X-Signature-Algorithm: hmac-sha256`

## Custom Headers

You can add custom HTTP headers to any endpoint:

```bash
WEBHOOK_ENDPOINT1_HEADERS=X-Custom-Header:value1,X-Another:value2
```

## Multiple Endpoints Example

Send notifications to multiple platforms simultaneously:

```bash
# Enable webhooks
WEBHOOK_ENABLED=true
WEBHOOK_ENDPOINTS=discord,slack,teams

# Discord
WEBHOOK_DISCORD_URL=https://discord.com/api/webhooks/...
WEBHOOK_DISCORD_FORMAT=discord

# Slack
WEBHOOK_SLACK_URL=https://hooks.slack.com/services/...
WEBHOOK_SLACK_FORMAT=slack
WEBHOOK_SLACK_AUTH_TYPE=bearer
WEBHOOK_SLACK_AUTH_TOKEN=xoxb-...

# Teams
WEBHOOK_TEAMS_URL=https://outlook.office.com/webhook/...
WEBHOOK_TEAMS_FORMAT=teams
```

## Debug Logging

Webhook implementation includes extensive debug logging at every step:

### Enable Debug Logging
```bash
DEBUG_LEVEL=debug  # or set to appropriate log level
```

### Debug Output Example
```
[DEBUG] WebhookNotifier initialization starting...
[DEBUG] Configuration: enabled=true, endpoints=2, default_format=discord, timeout=30s
[DEBUG] Endpoint #1: name=discord-alerts, url=https://discord.com/***MASKED***, format=discord
[DEBUG] HTTP client created with 30s timeout
[INFO]  ✅ WebhookNotifier initialized successfully with 2 endpoint(s)

[DEBUG] === WebhookNotifier.Send() called ===
[DEBUG] Processing 2 webhook endpoint(s)
[DEBUG] Notification data: status=success, hostname=pbs1.tis24.it, backup_size=7.2 MiB

[DEBUG] --- Processing endpoint 1/2: 'discord-alerts' ---
[DEBUG] buildDiscordPayload() - starting
[DEBUG] Status: success, color: 3066993 (green)
[DEBUG] Built 12 fields for Discord embed
[DEBUG] Payload built: 1847 bytes
[DEBUG] Creating HTTP POST request to https://discord.com/***MASKED***
[DEBUG] Sending HTTP POST request...
[DEBUG] Received HTTP 204 in 342ms
[INFO]  ✅ Webhook 'discord-alerts' sent successfully in 342ms

[DEBUG] === WebhookNotifier.Send() complete ===
[DEBUG] Results: success=2, failure=0, total_duration=863ms
```

## Error Handling

### Non-Blocking Failures
- Webhook failures do NOT abort the backup
- Errors are logged but the backup completes normally
- At least one successful endpoint = overall success

### Retry Logic
- Retries on HTTP 5xx (server errors)
- No retry on HTTP 4xx (client errors like 400, 401, 403, 404)
- Special handling for HTTP 429 (rate limiting) with 10s delay

### Rate Limiting
If a webhook returns HTTP 429:
1. Wait 10 seconds (longer than normal retry delay)
2. Retry the request
3. If still rate limited, fail the endpoint but continue others

## Implementation Details

### File Structure
```
internal/notify/
├── webhook.go              # Core WebhookNotifier implementation
├── webhook_payloads.go     # Format builders (Discord, Slack, Teams, Generic)
└── webhook_test.go         # Unit tests

internal/config/
└── config.go               # Webhook configuration parsing

internal/orchestrator/
└── notification_adapter.go # BackupStats → NotificationData conversion
```

### Code Statistics
- **New Code**: ~1000 lines across 3 files
- **Modified Code**: ~100 lines in 3 files
- **Configuration**: ~200 lines for parsing and building webhook config
- **Debug Logging**: ~150 lines of comprehensive debug output

## Testing

### Manual Testing with Discord

1. Create a Discord webhook in your server:
   - Server Settings → Integrations → Webhooks → New Webhook
   - Copy the webhook URL

2. Configure environment:
   ```bash
   export WEBHOOK_ENABLED=true
   export WEBHOOK_ENDPOINTS=discord
   export WEBHOOK_DISCORD_URL=https://discord.com/api/webhooks/YOUR_URL
   export WEBHOOK_DISCORD_FORMAT=discord
   export DEBUG_LEVEL=debug
   ```

3. Run backup:
   ```bash
   ./build/proxmox-backup --log-level=debug
   ```

4. Check Discord channel for notification

### Verifying Debug Output

With `DEBUG_LEVEL=debug`, you should see:
- Initialization messages with endpoint configuration
- Payload building with byte counts
- HTTP request details (masked URLs)
- Response status codes and durations
- Success/failure summaries

## Troubleshooting

### Webhook Not Sending

**Check:**
1. `WEBHOOK_ENABLED=true` is set
2. At least one endpoint is configured in `WEBHOOK_ENDPOINTS`
3. Endpoint URL is correct and accessible
4. Authentication credentials are valid

**Debug:**
```bash
export DEBUG_LEVEL=debug
./build/proxmox-backup 2>&1 | grep -i webhook
```

### HTTP 403 Forbidden

**Causes:**
- Invalid webhook URL
- Webhook was deleted or disabled
- Wrong authentication credentials (if using bearer/basic/hmac)

**Fix:**
- Verify webhook URL is still valid
- Regenerate webhook in Discord/Slack/Teams
- Check authentication configuration

### HTTP 400 Bad Request

**Causes:**
- Invalid payload format
- Missing required fields
- JSON encoding errors

**Fix:**
- Check debug logs for payload preview
- Verify format matches platform (discord/slack/teams)
- Report issue with full debug output

### Rate Limiting (HTTP 429)

**Causes:**
- Too many requests to webhook endpoint
- Platform rate limits exceeded

**Fix:**
- Wait for rate limit to reset
- Reduce backup frequency
- Use multiple webhooks to distribute load

## Security Considerations

1. **URL Masking**: All webhook URLs are masked in logs after the hostname
2. **Header Masking**: Authentication headers are masked in debug output
3. **HMAC Signatures**: Payload integrity verification for sensitive endpoints
4. **HTTPS Required**: Always use HTTPS webhook URLs in production
5. **Token Rotation**: Rotate webhook URLs and tokens regularly

## Integration with Existing Notifications

Webhooks work alongside Telegram and Email notifications:

```bash
# All notification methods enabled
TELEGRAM_ENABLED=true
EMAIL_ENABLED=true
WEBHOOK_ENABLED=true

# Each method operates independently
# Failures in one method don't affect others
```

## Performance

- **Typical Latency**: 200-500ms per endpoint
- **Parallel Execution**: All endpoints are processed sequentially within a single goroutine
- **Timeout**: Configurable (default 30s)
- **Non-Blocking**: Notifications run after backup completion

## Future Enhancements

Potential improvements for future versions:

- [ ] Parallel endpoint execution for faster notifications
- [ ] Custom templates for generic format
- [ ] Attachment support (send log files)
- [ ] Conditional notifications (only on errors)
- [ ] Notification grouping/batching
- [ ] Prometheus metrics for webhook success/failure rates

## Implementation Summary

**Phase 5.2 Completed:**
- ✅ Core webhook implementation with 4 formats
- ✅ Configuration parsing from environment variables
- ✅ Authentication support (Bearer, Basic, HMAC)
- ✅ Custom headers support
- ✅ Multiple endpoint support
- ✅ Extensive debug logging
- ✅ Non-blocking error handling
- ✅ Retry logic with rate limiting support
- ✅ Integration with orchestrator
- ✅ Build verification successful

**Lines of Code:**
- New: ~1000 lines
- Modified: ~100 lines
- Total: ~1100 lines changed

**Files Modified:**
- Created: 2 files (webhook.go, webhook_payloads.go)
- Modified: 3 files (config.go, main.go, notification_adapter.go)
