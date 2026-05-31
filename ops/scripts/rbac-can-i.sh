#!/usr/bin/env bash
set -euo pipefail

namespace="${1:-default}"

for verb in get list watch create update patch delete; do
  printf '%-8s pods: ' "$verb"
  kubectl auth can-i "$verb" pods -n "$namespace"
done

for resource in deployments services configmaps secrets roles rolebindings; do
  printf '%-16s list: ' "$resource"
  kubectl auth can-i list "$resource" -n "$namespace"
done
