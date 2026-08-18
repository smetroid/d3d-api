#!/bin/sh
set -e
export BIND_ADDR="0.0.0.0:${PORT:-8081}"
export POSTGRES_DSN="${DATABASE_URL}"
gomplate --input samus.tmpl --output samus.toml
exec ./main --config ./samus.toml
