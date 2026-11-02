#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib.sh"
require_context

k -n vault exec vault-0 -- vault status || true
