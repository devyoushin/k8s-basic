#!/usr/bin/env bash
set -euo pipefail

section() {
  printf '\n== %s ==\n' "$1"
}

section "context"
kubectl config current-context

section "version"
kubectl version --short 2>/dev/null || kubectl version

section "nodes"
kubectl get nodes -o wide

section "namespaces"
kubectl get namespaces

section "system pods"
kubectl get pods -n kube-system -o wide

section "component health"
kubectl get componentstatuses 2>/dev/null || echo "componentstatuses is deprecated or unavailable"
