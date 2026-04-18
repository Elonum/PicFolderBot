# Alerts Runbook

This file describes what to do when alerts appear in the Telegram alert channel.

## Alert Format

Each alert includes:
- severity (`ALERT` or `CRITICAL`)
- time (RFC3339)
- service (`PicFolderBot`)
- request_id / update_id (for Telegram-flow correlation, when available)
- component/op/status/path/error
- user_id (when available from Telegram update context)

## Main Alert Types

### 1) `yadisk authorization error`
- Meaning: Yandex Disk token is invalid/expired or access is denied (`401/403`).
- Impact: users cannot list/upload files.
- Actions:
  1. Verify `YANDEX_OAUTH_TOKEN` in `.env`.
  2. Re-issue OAuth token with required scopes.
  3. Restart service.
  4. Run `/upload` smoke test.

### 2) `yadisk unstable upstream or network`
- Meaning: transient network failures, timeouts, 5xx, or throttling conditions.
- Impact: partial delays/failures in listing/upload flow.
- Actions:
  1. Check host internet/DNS reachability to `cloud-api.yandex.net`.
  2. Check provider outage status.
  3. Verify repeated alerts are not a temporary spike.
  4. If persistent, increase network diagnostics and check server firewall/proxy.

### 3) `yadisk retries exhausted by transient status`
- Meaning: all retry attempts were consumed for 429/5xx.
- Impact: operation failed even with retries.
- Actions:
  1. Check Yandex API status and request rate.
  2. Inspect logs around retries/attempts.
  3. If 429 is frequent, reduce burst activity and evaluate queue/backoff tuning.

### 4) `panic in update handler` / other `CRITICAL`
- Meaning: unexpected runtime fault in bot flow.
- Impact: request dropped; potentially unstable runtime.
- Actions:
  1. Inspect full stack context in logs near alert timestamp.
  2. Identify user flow from `user_id` and reproduce.
  3. Fix and deploy hot patch.

## Health/Readiness Checks

- `GET /healthz` -> liveness (process responds)
- `GET /readyz` -> readiness (service is ready to serve)
- `GET /debug/vars` -> expvar counters

## Shutdown Behavior

On SIGTERM:
- bot stops receiving updates;
- uploader queue drains until completion or `SHUTDOWN_TIMEOUT_SEC` timeout.

If timeout is reached, warning is logged and process exits.
