#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
BRIDGE_ENV_FILE="${PROJECT_ROOT}/k8s/dev-setup/k8s-bridge.env"

BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info()  { echo -e "${BLUE}[INFO]${NC} $*"; }
log_ok()    { echo -e "${GREEN}[OK]${NC}   $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_err()   { echo -e "${RED}[ERR]${NC}  $*" >&2; }

FORCE_RESTART=false
if [[ "${1:-}" == "--force" ]]; then
    FORCE_RESTART=true
fi

# 1. Detect current primary host IPv4 address (default route source IP)
HOST_IP="$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src") {print $(i+1); exit}}')"
if [[ -z "${HOST_IP}" ]]; then
    HOST_IP="$(hostname -I | awk '{print $1}')"
fi

# 2. Check current Kubernetes API endpoint IP and ClusterIP reachability
K8S_ENDPOINT_IP="$(kubectl get endpoints kubernetes -n default -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null || true)"
CLUSTER_IP="$(kubectl get svc kubernetes -n default -o jsonpath='{.spec.clusterIP}' 2>/dev/null || echo '10.152.183.1')"

log_info "Current host IP:              ${HOST_IP:-unknown}"
log_info "Kubernetes endpoint IP:       ${K8S_ENDPOINT_IP:-unreachable}"
log_info "Kubernetes service ClusterIP: ${CLUSTER_IP}"

APISERVER_VIP_OK=false
if curl -ksS --max-time 3 "https://${CLUSTER_IP}:443/livez" >/dev/null 2>&1; then
    APISERVER_VIP_OK=true
fi

# 3. Restart Canonical k8s snap if the IP changed, VIP is unreachable, or --force was passed
if [[ "${FORCE_RESTART}" == "true" ]] || [[ "${HOST_IP}" != "${K8S_ENDPOINT_IP}" ]] || [[ "${APISERVER_VIP_OK}" != "true" ]]; then
    log_warn "Network change or stale Kubernetes endpoint detected. Restarting Canonical Kubernetes (snap k8s)..."
    sudo snap restart k8s

    log_info "Waiting for kube-apiserver to report endpoint ${HOST_IP}..."
    for i in {1..45}; do
        K8S_ENDPOINT_IP="$(kubectl get endpoints kubernetes -n default -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null || true)"
        if [[ -n "${K8S_ENDPOINT_IP}" ]] && [[ "${K8S_ENDPOINT_IP}" == "${HOST_IP}" ]]; then
            log_ok "Kubernetes endpoint updated to ${K8S_ENDPOINT_IP}"
            break
        fi
        sleep 2
        if [[ $i -eq 45 ]]; then
            log_warn "Kubernetes endpoint is '${K8S_ENDPOINT_IP:-empty}' (expected '${HOST_IP}'), continuing with CNI recovery..."
        fi
    done

    # 4. Restart Cilium, Istio CNI, and Ztunnel daemonsets so eBPF & CNI pick up the new endpoint
    log_info "Restarting Cilium, Istio CNI, and Ztunnel DaemonSets..."
    kubectl rollout restart daemonset/cilium -n kube-system
    kubectl rollout restart daemonset/istio-cni-node -n istio-system
    kubectl rollout restart daemonset/ztunnel -n istio-system

    log_info "Waiting for DaemonSets to roll out..."
    kubectl rollout status daemonset/cilium -n kube-system --timeout=120s
    kubectl rollout status daemonset/istio-cni-node -n istio-system --timeout=120s
    kubectl rollout status daemonset/ztunnel -n istio-system --timeout=120s
    log_ok "All CNI and mesh DaemonSets are running"
else
    log_ok "Host IP (${HOST_IP}) matches Kubernetes endpoint and ClusterIP (${CLUSTER_IP}) is reachable (pass --force to restart anyway)"
fi

# 5. Verify ClusterIP reachability after recovery
if curl -ksS --max-time 5 "https://${CLUSTER_IP}:443/livez" >/dev/null 2>&1; then
    log_ok "Kubernetes ClusterIP https://${CLUSTER_IP}:443/livez is reachable"
else
    log_err "Kubernetes ClusterIP https://${CLUSTER_IP}:443/livez is still unreachable"
    exit 1
fi

# 6. Check and sync k8s/dev-setup/k8s-bridge.env if the bridge IP changed
BRIDGE_IP="$(ip -4 -o addr show | awk '/cilium_host|cni0|flannel\.1/ {split($4, a, "/"); print a[1]; exit}')"
if [[ -n "${BRIDGE_IP}" ]] && [[ -f "${BRIDGE_ENV_FILE}" ]]; then
    CURRENT_BRIDGE_ENV="$(cat "${BRIDGE_ENV_FILE}" 2>/dev/null || true)"
    if [[ "${CURRENT_BRIDGE_ENV}" != "K8S_BRIDGE_IP=${BRIDGE_IP}" ]]; then
        echo "K8S_BRIDGE_IP=${BRIDGE_IP}" > "${BRIDGE_ENV_FILE}"
        log_ok "Updated ${BRIDGE_ENV_FILE} to K8S_BRIDGE_IP=${BRIDGE_IP}"
    else
        log_ok "${BRIDGE_ENV_FILE} is already up to date (K8S_BRIDGE_IP=${BRIDGE_IP})"
    fi
fi

log_ok "Network recovery complete!"
