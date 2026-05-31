#!/usr/bin/env bash
set -euo pipefail

namespace="${1:-default}"
pod="${2:-}"

if [[ -z "$pod" ]]; then
  echo "usage: $0 <namespace> <pod>"
  exit 2
fi

section() {
  printf '\n== %s ==\n' "$1"
}

section "pod"
kubectl get pod "$pod" -n "$namespace" -o wide

section "owners"
kubectl get pod "$pod" -n "$namespace" -o jsonpath='{.metadata.ownerReferences}{"\n"}'

section "describe"
kubectl describe pod "$pod" -n "$namespace"

section "logs"
kubectl logs "$pod" -n "$namespace" --all-containers --tail=100 || true

section "previous logs"
kubectl logs "$pod" -n "$namespace" --all-containers --previous --tail=100 || true
