#!/bin/sh
set -eu

# Air v1.63.4 does not load dotenv files. Run from be/.
# Export values so the server inherits them on every restart.
if [ -f .env ]; then
  set -a
  . ./.env
  set +a
fi

exec ./tmp/server "$@"
