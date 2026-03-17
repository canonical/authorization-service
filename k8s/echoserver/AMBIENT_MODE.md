# Ambient Mode Authorization Configuration

This document explains how echoserver is protected by external authorization in Istio ambient mode.

## Architecture

In Istio **ambient mode**, L7 policies like `AuthorizationPolicy` require a **waypoint proxy**. Here's how it works:

```
User Request → Istio Gateway → ztunnel (L4) → Waypoint Proxy (L7)
                                                      ↓
                                              AuthorizationPolicy
                                                      ↓
                                              External Authz Check
                                                      ↓
                                              Authorization Service
                                                      ↓
                                              Echoserver
```

## Components

### 1. Waypoint Proxy (`waypoint.yaml`)

**Purpose:** Enables L7 processing in ambient mode

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: echoserver-waypoint
  labels:
    istio.io/waypoint-for: service  # Service-level waypoint
spec:
  gatewayClassName: istio-waypoint
  listeners:
  - name: mesh
    port: 15008
    protocol: HBONE  # HTTP-Based Overlay Network Environment
```

**Key Points:**
- `istio-waypoint` gateway class is specific to ambient mode
- `HBONE` protocol is used for secure L7 processing
- Label `istio.io/waypoint-for: service` makes this a service waypoint

### 2. Service Label

The `echoserver` service is labeled to use the waypoint:

```yaml
metadata:
  labels:
    istio.io/use-waypoint: echoserver-waypoint
```

This tells Istio to route traffic through the waypoint proxy for L7 processing.

### 3. External Authorization Provider

Configured in `helm/ext_authz.yaml`:

```yaml
extensionProviders:
  - name: "cerberus"
    envoyExtAuthzGrpc:
      service: "authorization-service.default.svc.cluster.local"
      port: "9090"
```

### 4. Authorization Policies

**Policy 1: `echoserver-ext-authz`**
- **TargetRef:** `Gateway/echoserver-waypoint` (L7 enforcement at waypoint)
- **Action:** `CUSTOM` with provider `cerberus`
- **Rules:** All paths require external authorization except /health

**Policy 2: `echoserver-allow-health`**
- **TargetRef:** `Gateway/echoserver-waypoint`
- **Action:** `ALLOW`
- **Rule:** Public access to `/health` and `/echo/health`

**Key Change for Ambient Mode:**
- Uses `targetRef` to target the waypoint Gateway instead of `selector` to match workloads
- This ensures policies are enforced at the L7 waypoint proxy, not at the pod level


## How Authorization Works

1. **Request arrives** at Istio gateway
2. **ztunnel** captures L4 traffic (ambient node proxy)
3. **Traffic routed** to echoserver service
4. **Service label** triggers waypoint proxy routing
5. **Waypoint proxy** enforces L7 `AuthorizationPolicy`
6. **Custom provider** triggers external authorization
7. **Authorization service** validates session and returns JWT
8. **Request forwarded** to echoserver with JWT header

## Verification

### Check Waypoint Proxy

```bash
# Verify waypoint gateway is created
kubectl get gateway echoserver-waypoint -n default

# Check waypoint proxy pod
kubectl get pods -n default -l gateway.networking.k8s.io/gateway-name=echoserver-waypoint
```

### Check Service Label

```bash
kubectl get svc echoserver -o jsonpath='{.metadata.labels.istio\.io/use-waypoint}'
# Should output: echoserver-waypoint
```

### Check Authorization Policies

```bash
kubectl get authorizationpolicy -l example=echoserver-with-authz
# Should show: echoserver-ext-authz, echoserver-allow-health
```

### Test Authorization Flow

```bash
# 1. Test public health endpoint (should work)
curl http://localhost/echo/health

# 2. Test protected endpoint without auth (should fail with 401/403)
curl -v http://localhost/echo/

# 3. Test with valid session cookie (should work)
curl -H "Cookie: session-id=VALID_SESSION" http://localhost/echo/
```

## Why Waypoint is Needed

**Without waypoint proxy:**
- Only L4 policies work (basic allow/deny)
- No external authorization
- No custom headers
- No advanced routing

**With waypoint proxy:**
- ✅ Full L7 policy enforcement
- ✅ External authorization
- ✅ Header manipulation
- ✅ Advanced traffic management
- ✅ Same features as sidecar mode

## Deployment

The waypoint is automatically deployed with:

```bash
kubectl apply -k k8s/echoserver/
```

Or via Skaffold:

```bash
skaffold run -p echoserver
```

## Troubleshooting

### Waypoint not created

```bash
# Check if istio-waypoint GatewayClass exists
kubectl get gatewayclass istio-waypoint

# Create if missing (should be installed with Istio)
```

### Authorization not working

```bash
# Check waypoint proxy logs
kubectl logs -n default -l gateway.networking.k8s.io/gateway-name=echoserver-waypoint

# Verify external authz provider
kubectl get configmap -n istio-system istiod -o yaml | grep -A 10 extensionProviders
```

### Service not using waypoint

```bash
# Verify service has the label
kubectl get svc echoserver -o yaml | grep use-waypoint
```

## Summary

✅ **Waypoint proxy** enables L7 policies in ambient mode  
✅ **AuthorizationPolicy** with CUSTOM provider enforces external authz  
✅ **Service labeled** to use waypoint for L7 processing  
✅ **Full feature parity** with sidecar mode  
✅ **Lower resource usage** than sidecar injection  
