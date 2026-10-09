# Istio Ambient Mode Configuration

This directory contains Istio manifests for configuring ambient mode and ingress gateway for the authorization-service.

## Files

- **`namespace.yaml`** - Creates istio-system namespace (if needed)
- **`waypoint.yaml`** - Waypoint proxy Gateway for L7 traffic processing in the ambient mesh
- **`policies.yaml`** - AuthorizationPolicy allowing gRPC check calls (`/envoy.service.auth.v3.Authorization/Check`) through the waypoint
- **`gateway.yaml`** - Gateway resource defining the HTTP ingress gateway
- **`kustomization.yaml`** - Kustomize config to apply all resources

## Architecture

### Ambient Mode
- **No sidecars** - Uses ztunnel (node-level proxy) for L4 mTLS traffic
- **Waypoint proxy** - Handles L7 processing, Envoy external authorization filters, and AuthorizationPolicy enforcement
- **gRPC internal** - ExtAuthz gRPC service on port 9090 remains internal to the mesh
- **HTTP endpoints** - Health and metrics on port 8080

## Deployment

### Prerequisites

1. Istio installed in ambient mode:
   ```bash
   skaffold dev -p istio
   ```

2. Gateway API CRDs installed (usually included with Istio)

### Apply Manifests

```bash
# From project root
kubectl apply -k k8s/istio/
```

### Verify

```bash
# Check Gateway status
kubectl get gateway -n default

# Check Waypoint status
kubectl get gateway waypoint -n default

# Check AuthorizationPolicy
kubectl get authorizationpolicy -n default
```

## Troubleshooting

### Waypoint or Gateway not ready

```bash
# Check gateway status
kubectl describe gateway authorization-service-gateway -n default

# Check waypoint status
kubectl describe gateway waypoint -n default

# Check waypoint pods
kubectl get pods -n default -l gateway.networking.k8s.io/gateway-name=waypoint
```

### Traffic not reaching service

```bash
# Check service endpoints
kubectl get endpoints authorization-service -n default

# Verify pods are running
kubectl get pods -l app=authorization-service

# Check ztunnel logs
kubectl logs -n istio-system -l app=ztunnel
```
