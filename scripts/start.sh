#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
docker compose up --build -d
printf 'Student console: http://localhost:8080\n'
