#!/usr/bin/env bash
set -euo pipefail

namespace="${1:---all-namespaces}"

section() {
  printf '\n== %s ==\n' "$1"
}

section "deployments"
kubectl get deployments "$namespace" -o wide

section "statefulsets"
kubectl get statefulsets "$namespace" -o wide

section "daemonsets"
kubectl get daemonsets "$namespace" -o wide

section "pods not running"
kubectl get pods "$namespace" --field-selector=status.phase!=Running -o wide

section "recent events"
kubectl get events "$namespace" --sort-by=.lastTimestamp | tail -30
