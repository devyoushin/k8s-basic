#!/usr/bin/env bash
set -euo pipefail

if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl is required" >&2
  exit 1
fi

if [ "$(kubectl auth can-i get persistentvolumes)" != "yes" ] \
  || [ "$(kubectl auth can-i get persistentvolumeclaims --all-namespaces)" != "yes" ] \
  || [ "$(kubectl auth can-i get pods --all-namespaces)" != "yes" ]; then
  echo "Current identity needs get permission for PV, PVC, and Pod resources." >&2
  exit 1
fi

printf 'PV\tNAMESPACE\tPVC\tPOD\tNODE\tEFS_VOLUME_HANDLE\n'

while IFS=$'\t' read -r pv namespace pvc volume_handle; do
  [ -n "$namespace" ] && [ -n "$pvc" ] || continue

  kubectl get pods -A \
    -o jsonpath='{range .items[*]}{.metadata.namespace}{"\t"}{.metadata.name}{"\t"}{.spec.nodeName}{"\t"}{range .spec.volumes[*]}{.persistentVolumeClaim.claimName}{" "}{end}{"\n"}{end}' \
    | awk -F '\t' -v wanted_namespace="$namespace" -v wanted_pvc="$pvc" \
      -v pv="$pv" -v handle="$volume_handle" '
        $1 == wanted_namespace {
          count = split($4, claims, " ")
          for (i = 1; i <= count; i++) {
            if (claims[i] == wanted_pvc) {
              printf "%s\t%s\t%s\t%s\t%s\t%s\n", pv, $1, wanted_pvc, $2, $3, handle
              break
            }
          }
        }
      '
done < <(
  kubectl get pv \
    -o jsonpath='{range .items[?(@.spec.csi.driver=="efs.csi.aws.com")]}{.metadata.name}{"\t"}{.spec.claimRef.namespace}{"\t"}{.spec.claimRef.name}{"\t"}{.spec.csi.volumeHandle}{"\n"}{end}'
)
