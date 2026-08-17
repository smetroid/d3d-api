#!/bin/sh
set -e
export BIND_ADDR="0.0.0.0:${PORT:-8081}"
gomplate --input samus.tmpl --output samus.toml
exec ./main --config ./samus.toml
