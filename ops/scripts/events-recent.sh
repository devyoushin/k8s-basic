#!/usr/bin/env bash
set -euo pipefail

namespace="${1:---all-namespaces}"

kubectl get events "$namespace" --sort-by=.lastTimestamp | tail -100
