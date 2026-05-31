#!/usr/bin/env bash
set -euo pipefail

namespace="${1:-default}"
name="${2:-kubernetes.default.svc.cluster.local}"

kubectl run dns-check \
  --namespace "$namespace" \
  --image busybox:1.36 \
  --restart Never \
  --rm -i \
  --command -- nslookup "$name"
