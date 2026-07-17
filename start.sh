#!/usr/bin/env bash
set -e

cd "$(dirname "$0")"

: "${CDK_DATABASE_URL:?set CDK_DATABASE_URL before running start.sh}"
: "${CDK_REDIS_URL:?set CDK_REDIS_URL before running start.sh}"

export CDK_DATABASE_URL
export CDK_REDIS_URL
export CDK_HOST="${CDK_HOST:-127.0.0.1}"
export CDK_PORT="${CDK_PORT:-8080}"

# Ensure dist is next to the binary for SPA serving
if [ ! -d "bin/web/dist" ]; then
  mkdir -p bin/web
  cp -r web/dist bin/web/
fi

exec ./bin/api
