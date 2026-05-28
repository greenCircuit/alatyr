"""Test-wide constants: namespace names + base URL. Single source of truth so
conftest, scenarios, and tests stay in sync.
"""

import os

NS_A = "graph-test-a"
NS_B = "graph-test-b"
NS_C = "graph-test-c"
TEST_NAMESPACES = [NS_A, NS_B, NS_C]

BACKEND_URL = os.environ.get("BACKEND_URL", "http://localhost:8080")
