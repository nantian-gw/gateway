#!/usr/bin/env bash
set -euo pipefail

gateway_api_version="${GATEWAY_API_VERSION:-v1.5.1}"
apply_file="$(go env GOMODCACHE)/sigs.k8s.io/gateway-api/conformance@${gateway_api_version}/utils/kubernetes/apply.go"

if [[ ! -f "$apply_file" ]]; then
  echo "Gateway API conformance apply.go not found: $apply_file" >&2
  exit 1
fi

chmod u+w "$apply_file"

python3 - "$apply_file" <<'PYEOF'
import sys
from pathlib import Path

path = Path(sys.argv[1])
content = path.read_text()

old = '\t\tuObj.SetResourceVersion(fetchedObj.GetResourceVersion())\n\t\ttlog.Logf(t, "Updating %s %s", uObj.GetName(), uObj.GetKind())\n\t\terr = c.Update(ctx, uObj)'
new = '''\t\tuObj.SetResourceVersion(fetchedObj.GetResourceVersion())
\t\ttlog.Logf(t, "Updating %s %s", uObj.GetName(), uObj.GetKind())
\t\tfor i := 0; i < 5; i++ {
\t\t\tif i > 0 {
\t\t\t\ttime.Sleep(time.Duration(1<<i) * 100 * time.Millisecond)
\t\t\t\tlatest := &unstructured.Unstructured{}
\t\t\t\tlatest.SetGroupVersionKind(uObj.GroupVersionKind())
\t\t\t\tif getErr := c.Get(ctx, client.ObjectKeyFromObject(uObj), latest); getErr != nil {
\t\t\t\t\terr = getErr
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t\tuObj.SetResourceVersion(latest.GetResourceVersion())
\t\t\t}
\t\t\terr = c.Update(ctx, uObj)
\t\t\tif err == nil || !apierrors.IsConflict(err) {
\t\t\t\tbreak
\t\t\t}
\t\t}'''

already_patched = "for i := 0; i < 5; i++" in content and "apierrors.IsConflict(err)" in content
if not already_patched:
    if old not in content:
        raise SystemExit("expected Gateway API apply.go update block was not found")
    content = content.replace(old, new, 1)

if '\n\t"time"\n' not in content:
    if '"testing"' not in content:
        raise SystemExit("testing import was not found for time import insertion")
    content = content.replace('"testing"', '"testing"\n\t"time"', 1)

path.write_text(content)
print("Gateway API conformance apply.go patch is present")
PYEOF
