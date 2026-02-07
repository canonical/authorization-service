# Istio Ambient Mode Configuration

This directory contains Istio manifests for exposing the secure-token-service using Istio's ambient mode and Gateway API.

## Files

- **`namespace.yaml`** - Creates istio-system namespace (if needed)
- **`istio.yaml`** - Existing Istio configuration
- **`gateway.yaml`** - Gateway resource that creates an HTTP ingress gateway
- **`httproute.yaml`** - HTTPRoute that routes traffic from gateway to the service
- **`referencegrant.yaml`** - Allows cross-namespace references from istio-system to default
- **`kustomization.yaml`** - Kustomize config to apply all resources

## Architecture

### Ambient Mode
- **No sidecars** - Uses ztunnel (node-level proxy) for L4 traffic
- **Gateway API** - Modern Kubernetes ingress using Gateway and HTTPRoute
- **HTTP exposed** - Web endpoints available via gateway
- **gRPC internal** - gRPC service remains internal to the mesh

### Exposed HTTP Endpoints

Via the gateway, these endpoints are accessible:
- `/` - User info homepage
- `/auth/login` - OIDC login initiation
- `/auth/callback` - OIDC callback handler
- `/auth/logout` - Logout endpoint
- `/.well-known/jwks.json` - JSON Web Key Set
- `/metrics` - Prometheus metrics

### Internal Services

These remain internal (not exposed via gateway):
- gRPC service on port 9090

## Deployment

### Prerequisites

1. Istio installed in ambient mode:
   ```bash
   # Via skaffold (from project root)
   skaffold dev -p istio
   ```

2. Gateway API CRDs installed (usually included with Istio)

### Apply Manifests

```bash
# From project root
kubectl apply -k k8s/istio/

# Or individually
kubectl apply -f k8s/istio/
```

### Verify

```bash
# Check Gateway status
kubectl get gateway -n istio-system

# Check HTTPRoute status
kubectl get httproute -n default

# Check ReferenceGrant
kubectl get referencegrant -n default

# Get gateway external IP
kubectl get svc -n istio-system | grep sts-gateway
```

## Testing

### Get Gateway Address

```bash
# For LoadBalancer
GATEWAY_IP=$(kubectl get svc -n istio-system sts-gateway-istio -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

# For NodePort (local testing)
GATEWAY_PORT=$(kubectl get svc -n istio-system sts-gateway-istio -o jsonpath='{.spec.ports[0].nodePort}')
NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[0].address}')
```

### Test Endpoints

```bash
# Homepage
curl http://$GATEWAY_IP/

# JWKS endpoint
curl http://$GATEWAY_IP/.well-known/jwks.json

# Metrics
curl http://$GATEWAY_IP/metrics
```

## Configuration

### Custom Hostname

To use a specific hostname instead of wildcard `*`:

**gateway.yaml:**
```yaml
listeners:
- name: http
  hostname: "sts.example.com"  # Your domain
```

**httproute.yaml:**
```yaml
spec:
  hostnames:
  - "sts.example.com"  # Must match gateway
```

### HTTPS/TLS

To add TLS:

**gateway.yaml:**
```yaml
listeners:
- name: https
  hostname: "sts.example.com"
  port: 443
  protocol: HTTPS
  tls:
    mode: Terminate
    certificateRefs:
    - name: sts-tls-cert
      kind: Secret
```

Then create a TLS secret:
```bash
kubectl create secret tls sts-tls-cert \
  --cert=path/to/cert.pem \
  --key=path/to/key.pem \
  -n istio-system
```

## Troubleshooting

### Gateway not ready

```bash
# Check gateway status
kubectl describe gateway sts-gateway -n istio-system

# Check gateway pods
kubectl get pods -n istio-system -l gateway.networking.k8s.io/gateway-name=sts-gateway
```

### HTTPRoute not working

```bash
# Check route status
kubectl describe httproute sts-route -n default

# Verify service exists
kubectl get svc secure-token-service -n default

# Check ReferenceGrant
kubectl get referencegrant -n default
```

### Traffic not reaching service

```bash
# Check service endpoints
kubectl get endpoints secure-token-service -n default

# Verify pods are running
kubectl get pods -l app=secure-token-service

# Check ztunnel logs
kubectl logs -n istio-system -l app=ztunnel
```
