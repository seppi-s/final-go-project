#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
docker compose exec web python app.py "${1:-summary}"
