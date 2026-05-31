#!/usr/bin/env bash
set -euo pipefail

section() {
  printf '\n== %s ==\n' "$1"
}

section "nodes"
kubectl get nodes -o wide

section "node conditions"
kubectl get nodes -o custom-columns=NAME:.metadata.name,READY:.status.conditions[-1].status,VERSION:.status.nodeInfo.kubeletVersion,OS:.status.nodeInfo.osImage

section "taints"
kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.taints}{"\n"}{end}'

section "allocatable"
kubectl get nodes -o custom-columns=NAME:.metadata.name,CPU:.status.allocatable.cpu,MEMORY:.status.allocatable.memory,PODS:.status.allocatable.pods

section "top nodes"
kubectl top nodes 2>/dev/null || echo "metrics-server is unavailable"
