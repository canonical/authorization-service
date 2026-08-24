## 🚀 Development Guide - Authorization Service
 This document describes the procedures for setting up the local environment and managing the project lifecycle using Kubernetes, Istio (Ambient Mesh), and Skaffold.

### 🏗 Environment Startup (Skaffold)
To ensure Istio configuration patches are applied correctly and component dependencies are respected, Skaffold profiles must be started one at a time, in **separate terminals**, following this strict order:

1.  **Istio Core**
    ```bash
    skaffold dev -p istio
    ```
2.  **Dev Setup (Patch & ServiceEntry)**
    ```bash
    skaffold dev -p dev-setup
    ```
3.  **Echoserver (App & Policies)**
    ```bash
    skaffold dev -p echoserver
    ```

#### If you use Podman
In case you use podman, to make sure Skaffold behaves correctly, you need to do 2 things
1. enable the service that emulates docker
```shell
systemctl --user enable --now podman.socket
export DOCKER_HOST="unix://$XDG_RUNTIME_DIR/podman/podman.sock"
```

2. run skaffold with the podman driver option
```shell
skaffold dev --driver=podman -p <profile of choice>
```

---

## 🌐 Networking & Alias

Add these aliases to your shell profile (.bashrc or .zshrc) to easily manage dynamic cluster IP addresses.

### Gateway IP
Retrieve the Authorization-service Gateway IP address for sending HTTP requests.

```bash
alias authorization-service-gateway='kubectl get gateway authorization-service-gateway -n default -o jsonpath="{.status.addresses[0].value}"'
```

### K8s Bridge IP (for ServiceEntry)
Find the bridge interface IP to allow the cluster to reach services running on your local host.
```bash
alias k8s-bridge='ip addr show | grep -E "cni0|flannel.1|cilium_host" | grep inet | cut -d " " -f 6 | cut -d "/" -f 1'
```
 
After you have the IP, set it as the `K8S_BRIDGE_IP` environment variable in a file called `k8s-bridge.env` in the k8s/dev-setup directory.

---

## 🧪 Testing and invocation

The infrastructure uses Kubernetes Gateway APIs. To test the echoserver, you must satisfy the gateway requirements (Host header) and the authorization policy (session cookie).

### Curl
After retrieving the gateway IP, run the following test:

```bash
# Retrieve the current Gateway IP
GATEWAY_IP=$(authorization-service-gateway)

# Run the test (ensure your local auth app is running)
curl -I -H "Host: localhost" \
     --cookie "session=your_session_token_here" \
     http://$GATEWAY_IP/echo
```

> ### Important
> Header Note: Always use Host: localhost. Using Hostname or omitting the header will result in a 403 Forbidden error because the Gateway won't find a matching route.

---

## 📝 Technical Notes
MeshConfig Patch: The dev-setup profile patches the istio ConfigMap in the istio-system namespace. If the patch isn't applied, ensure the k8s/dev-setup/istio-mesh-base.yaml file is present as it serves as the base for Kustomize.

Waypoint Proxy: Layer 7 (L7) checks happen in the Waypoint Proxy. If you notice anomalous behavior or policies not being picked up, restart it:
```bash
kubectl rollout restart deployment waypoint -n default
```
