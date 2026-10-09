#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
docker compose exec web /app/student-console "${1:-summary}"
