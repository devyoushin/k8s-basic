#!/usr/bin/env bash
set -euo pipefail

section() {
  printf '\n== %s ==\n' "$1"
}

section "api resources"
kubectl api-resources

section "crds"
kubectl get crds 2>/dev/null || echo "no permission or no CRDs"

section "api versions"
kubectl api-versions
