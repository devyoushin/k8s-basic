#!/usr/bin/env bash
set -euo pipefail

namespace="${1:---all-namespaces}"

echo "== nodes =="
kubectl top nodes 2>/dev/null || echo "metrics-server is unavailable"

echo
echo "== pods =="
kubectl top pods "$namespace" 2>/dev/null || echo "metrics-server is unavailable"
