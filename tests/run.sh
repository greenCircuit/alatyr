#!/usr/bin/env bash
# Orchestrator: bootstrap a kwok apiserver, apply Istio CRDs, build + run
# the graph server against it, then drive Robot tests.
#
# First-run installs (kwok binary, Istio CRDs, python deps) are cached under
# tests/.bin and tests/.cache. Re-runs reuse the cache and skip downloads.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_DIR="$SCRIPT_DIR/.bin"
CACHE_DIR="$SCRIPT_DIR/.cache"
RESULTS_DIR="./"

KWOK_VERSION="${KWOK_VERSION:-v0.6.1}"
ISTIO_VERSION="${ISTIO_VERSION:-1.22.0}"
CLUSTER_NAME="${CLUSTER_NAME:-graph-tests}"
BACKEND_PORT="${BACKEND_PORT:-8080}"
BACKEND_READY_TIMEOUT="${BACKEND_READY_TIMEOUT:-30}"

mkdir -p "$BIN_DIR" "$CACHE_DIR" "$RESULTS_DIR"
export PATH="$BIN_DIR:$PATH"

log() { printf '\033[1;34m==>\033[0m %s\n' "$*" >&2; }
err() { printf '\033[1;31m!!!\033[0m %s\n' "$*" >&2; }

detect_platform() {
    local os arch
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64) arch=amd64 ;;
        aarch64|arm64) arch=arm64 ;;
        *) err "unsupported arch: $arch"; exit 1 ;;
    esac
    echo "${os}-${arch}"
}

ensure_kwok() {
    # Already on PATH (e.g. baked into CI container) — no-op.
    if command -v kwok >/dev/null 2>&1 && command -v kwokctl >/dev/null 2>&1; then
        return
    fi
    if [[ -x "$BIN_DIR/kwok" && -x "$BIN_DIR/kwokctl" ]]; then
        return
    fi
    local platform
    platform="$(detect_platform)"
    log "downloading kwok ${KWOK_VERSION} (${platform})"
    curl -fsSL -o "$BIN_DIR/kwok" \
        "https://github.com/kubernetes-sigs/kwok/releases/download/${KWOK_VERSION}/kwok-${platform}"
    curl -fsSL -o "$BIN_DIR/kwokctl" \
        "https://github.com/kubernetes-sigs/kwok/releases/download/${KWOK_VERSION}/kwokctl-${platform}"
    chmod +x "$BIN_DIR/kwok" "$BIN_DIR/kwokctl"
}

ensure_istio_crds() {
    # Prebaked into the CI image — Dockerfile sets this env var.
    if [[ -n "${ISTIO_CRDS_FILE:-}" && -f "$ISTIO_CRDS_FILE" ]]; then
        echo "$ISTIO_CRDS_FILE"
        return
    fi
    local target="$CACHE_DIR/istio-crds.yaml"
    if [[ -f "$target" ]]; then
        echo "$target"
        return
    fi
    log "downloading Istio CRDs ${ISTIO_VERSION}"
    curl -fsSL -o "$target" \
        "https://raw.githubusercontent.com/istio/istio/${ISTIO_VERSION}/manifests/charts/base/crds/crd-all.gen.yaml"
    echo "$target"
}

ensure_python_deps() {
    if ! command -v pytest >/dev/null 2>&1 || ! python3 -c "import kubernetes" >/dev/null 2>&1; then
        log "installing python deps"
        python3 -m pip install --quiet -r "$SCRIPT_DIR/requirements.txt"
    fi
}

build_backend() {
    log "building backend binary"
    (cd "$REPO_ROOT" && go build -o "$CACHE_DIR/graph-server" .)
}

# port_in_use returns 0 (true) when something is already listening on $1.
port_in_use() {
    python3 - "$1" <<'PY'
import socket, sys
sock = socket.socket()
busy = sock.connect_ex(("127.0.0.1", int(sys.argv[1]))) == 0
sock.close()
sys.exit(0 if busy else 1)
PY
}

# pick_free_port bumps BACKEND_PORT past any in-use port (devcontainer dev
# server commonly squats 8080), so a busy port no longer wedges the run.
pick_free_port() {
    local attempts=0
    while port_in_use "$BACKEND_PORT"; do
        log "port ${BACKEND_PORT} in use, trying $((BACKEND_PORT + 1))"
        BACKEND_PORT=$((BACKEND_PORT + 1))
        attempts=$((attempts + 1))
        if (( attempts >= 20 )); then
            err "no free port found near ${BACKEND_PORT}"; exit 1
        fi
    done
}

BACKEND_PID=""
cleanup() {
    log "cleanup"
    if [[ -n "$BACKEND_PID" ]] && kill -0 "$BACKEND_PID" 2>/dev/null; then
        kill "$BACKEND_PID" 2>/dev/null || true
        wait "$BACKEND_PID" 2>/dev/null || true
    fi
    kwokctl delete cluster --name "$CLUSTER_NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

RUN_PERF=false
PYTEST_ARGS=()
parse_args() {
    for arg in "$@"; do
        case "$arg" in
            --perf) RUN_PERF=true ;;
            *) PYTEST_ARGS+=("$arg") ;;
        esac
    done
}

main() {
    parse_args "$@"
    if ! command -v go >/dev/null 2>&1; then
        err "go not found in PATH"; exit 1
    fi
    if ! command -v kubectl >/dev/null 2>&1; then
        err "kubectl not found in PATH"; exit 1
    fi
    if ! command -v python3 >/dev/null 2>&1; then
        err "python3 not found in PATH"; exit 1
    fi

    ensure_kwok
    ensure_python_deps
    local crd_file
    crd_file="$(ensure_istio_crds)"
    build_backend

    log "creating kwok cluster '${CLUSTER_NAME}'"
    kwokctl delete cluster --name "$CLUSTER_NAME" >/dev/null 2>&1 || true
    kwokctl create cluster --name "$CLUSTER_NAME" --runtime=binary >/dev/null
    KUBECONFIG_FILE="$CACHE_DIR/kubeconfig"
    kwokctl get kubeconfig --name "$CLUSTER_NAME" > "$KUBECONFIG_FILE"
    export KUBECONFIG="$KUBECONFIG_FILE"

    log "applying Istio CRDs"
    kubectl apply -f "$crd_file" >/dev/null
    # Wait for the CRDs the istio engine watches to reach Established before
    # the backend's informers spin up. Without this, the discovery probe in
    # NewInformerClient can win the race against CRD registration; informers
    # then hold an empty cache forever and every istio test sees zero policies.
    kubectl wait --for=condition=Established \
        crd/authorizationpolicies.security.istio.io \
        crd/peerauthentications.security.istio.io \
        --timeout=30s >/dev/null
    sleep 10
    pick_free_port
    log "starting backend on port ${BACKEND_PORT}"
    # Strip DEMO_MODE explicitly — devcontainer shells often export it for
    # local dev, which would bypass the real KUBECONFIG path. BACKEND_PORT is
    # honored by main.go so a busy default port self-heals.
    env -u DEMO_MODE BACKEND_PORT="$BACKEND_PORT" KUBECONFIG="$KUBECONFIG_FILE" "$CACHE_DIR/graph-server" \
        > "$CACHE_DIR/backend.log" 2>&1 &
    BACKEND_PID=$!

    log "waiting for backend ready"
    local attempt=0
    until curl -fsS "http://localhost:${BACKEND_PORT}/api/cluster-state" >/dev/null 2>&1; do
        attempt=$((attempt + 1))
        if (( attempt >= BACKEND_READY_TIMEOUT )); then
            err "backend failed to become ready in ${BACKEND_READY_TIMEOUT}s"
            err "see $CACHE_DIR/backend.log"
            tail -n 40 "$CACHE_DIR/backend.log" >&2 || true
            exit 1
        fi
        sleep 1
    done

    # --perf swaps the functional suite for the initial-load latency sweep
    # (-m perf overrides the default "-m not perf" in pytest.ini).
    local marker_args=()
    local report_html="report.html"
    local report_junit="junit.xml"
    if [[ "$RUN_PERF" == "true" ]]; then
        # -m perf overrides the default "-m not perf" in pytest.ini; -s lets the
        # sweep's summary table print to the console.
        marker_args=(-m perf -s)
        report_html="perf-report.html"
        report_junit="perf-junit.xml"
        log "running pytest (perf sweep)"
    else
        log "running pytest"
    fi

    set +e
    (
        cd "$SCRIPT_DIR"
        export BACKEND_URL="http://localhost:${BACKEND_PORT}"
        export PERF_RESULTS_DIR="$RESULTS_DIR"
        pytest \
            --html="$RESULTS_DIR/${report_html}" \
            --self-contained-html \
            --junitxml="$RESULTS_DIR/${report_junit}" \
            "${marker_args[@]}" \
            "${PYTEST_ARGS[@]}" \
            -vv
    )
    local rc=$?
    set -e

    log "results: $RESULTS_DIR/report.html"
    exit "$rc"
}

main "$@"
