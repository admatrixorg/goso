#!/bin/sh
# Optional live channel smoke. Skip (exit 0) when flags/tokens are unset.
# Does not bind or kill :8082 :8091 :3000 :18080 :18088.
# No product secrets in this file.
set -eu

if [ -z "${GOSO_LIVE_CHANNELS:-}${GOSO_LIVE_TELEGRAM:-}${GOSO_LIVE_ZALO_OA:-}${GOSO_LIVE_ZALO_PERSONAL:-}" ]; then
  echo "e2e-channels-live: skip (no GOSO_LIVE_* flag)"
  exit 0
fi

if [ -n "${GOSO_LIVE_TELEGRAM:-}${GOSO_LIVE_CHANNELS:-}" ] && [ -z "${GOSO_TELEGRAM_BOT_TOKEN:-}" ]; then
  echo "e2e-channels-live: skip telegram (token unset)"
  exit 0
fi

if [ -n "${GOSO_LIVE_ZALO_OA:-}${GOSO_LIVE_CHANNELS:-}" ] && [ -z "${GOSO_ZALO_OA_ACCESS_TOKEN:-}" ]; then
  echo "e2e-channels-live: skip zalo-oa (token unset)"
  exit 0
fi

echo "e2e-channels-live: flags set; operator must run an ephemeral gateway --port 0 locally"
echo "e2e-channels-live: this script does not Dial vendors from CI"
exit 0
