"""Test-wide constants: namespace names + base URL. Single source of truth so
conftest, scenarios, and tests stay in sync.
"""

import os

NS_A = "graph-test-a"
NS_B = "graph-test-b"
NS_C = "graph-test-c"
TEST_NAMESPACES = [NS_A, NS_B, NS_C]

# Ambient-enrolled namespace for mesh tests. Kept separate from TEST_NAMESPACES
# so the existing unenrolled authz/edge/status-key suites are unaffected.
NS_MESH = "graph-test-mesh"

# Istio mesh root namespace. Mesh-level (selectorless) PeerAuthentications here
# apply cluster-wide; per-test teardown must wipe it even though it is not a
# test-owned namespace (never created/deleted, only policy-wiped).
ROOT_NS = "istio-system"

# Every namespace whose policies a per-test wipe must clear.
POLICY_NAMESPACES = TEST_NAMESPACES + [NS_MESH, ROOT_NS]

# Ambient-mesh enrollment label (see internal/mesh/istio/istio.go).
AMBIENT_LABEL_KEY = "istio.io/dataplane-mode"
AMBIENT_LABEL_VALUE = "ambient"

# ztunnel inbound HBONE port — NetworkPolicies on ambient workloads must allow
# it or mesh traffic is dropped. ValidateExternalRules flags its absence.
ZTUNNEL_HBONE_PORT = 15008

BACKEND_URL = os.environ.get("BACKEND_URL", "http://localhost:8080")

# Prometheus scrape endpoint. Served on a separate listener (main.go) so scrape
# traffic bypasses Recover + logging + HTTP-metrics middleware. Default 8085
# matches main.go; run.sh bumps it when 8085 is busy and exports METRICS_URL.
METRICS_URL = os.environ.get("METRICS_URL", "http://localhost:8085")
