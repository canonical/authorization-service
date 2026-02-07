# Echoserver with Istio External Authorization (Ambient Mode)

This example demonstrates how to protect an application (echoserver) using Istio's external authorization feature with the authorization-service in **ambient mode** (sidecar-less).

## Architecture

```
User Request → Istio Ingress Gateway → ztunnel → Waypoint Proxy (L7)
                                                         ↓
                                                 External Authorization Check
                                                         ↓
                                                 Authorization Service
                                                         ↓
                                                 STS (Session → JWT)
                                                         ↓
                                                 Allow/Deny Response
                                                         ↓
                                                 Echoserver (if allowed)
```

**Ambient Mode Benefits:**
- No sidecar injection required
- Lower resource overhead
- Simplified operations
- Automatic mesh enrollment at namespace level

## Prerequisites

1. **Kubernetes cluster** with **Istio ambient mode** installed
2. **Namespace labeled for ambient mesh**:
   ```bash
   kubectl label namespace default istio.io/dataplane-mode=ambient
   ```
3. **Authorization service** deployed and running:
   ```bash
   kubectl apply -k k8s/
   ```
4. **External authorization provider** configured in Istio (see `k8s/istio/istio-authz-policy.yaml`)
5. **STS service** deployed and accessible by the authorization service

## Deployment

**Important:** Ensure your namespace is enrolled in the ambient mesh:

```bash
kubectl label namespace default istio.io/dataplane-mode=ambient
```

Deploy the echoserver with authorization protection:

```bash
kubectl apply -k k8s/echoserver/
```

Verify the deployment:

```bash
kubectl get pods -l app=echoserver
kubectl get svc echoserver
kubectl get authorizationpolicy echoserver-ext-authz
```

**Note:** In ambient mode, you'll see only 1/1 container (no Istio sidecar). Traffic is handled by the ztunnel and waypoint proxy.

## Gateway Exposure

The echoserver is exposed on the `/echo` path via the Istio gateway (`sts-gateway`):

- **Gateway:** `sts-gateway` (in istio-system namespace)
- **Path:** `/echo` (PathPrefix match)
- **Hostnames:** `localhost`, `iam.internal.io`

This allows external access through the Istio ingress gateway while still enforcing authorization policies.

## Waypoint Proxy (Required for Ambient Mode L7 Policies)

In Istio ambient mode, **L7 policies like AuthorizationPolicy require a waypoint proxy**. The waypoint handles:
- External authorization checks
- Header manipulation
- Advanced routing rules

**Waypoint Configuration:**
- **Gateway:** `echoserver-waypoint` (service-level waypoint)
- **Protocol:** HBONE (HTTP-Based Overlay Network Environment)
- **Service Label:** `istio.io/use-waypoint: echoserver-waypoint`

The waypoint proxy is automatically created when you deploy with this configuration. See [AMBIENT_MODE.md](AMBIENT_MODE.md) for detailed architecture.

## Authorization Policy

The deployment includes two authorization policies:

### 1. Public Health Endpoint
- Path: `/health`
- Method: `GET`
- **No authentication required** - allows readiness/liveness probes and health checks

### 2. Protected Endpoints
- All other paths (`/*`)
- **Requires external authorization** via the authorization-service
- Session cookie must be present: `Cookie: session-id=<token>`
- Authorization service exchanges session for JWT with STS
- JWT is forwarded to echoserver as `Authorization: Bearer <jwt>`

## Testing

### 1. Test Public Endpoint (No Auth Required)

```bash
# Port-forward to echoserver
kubectl port-forward svc/echoserver 8080:80

# Access health endpoint - should succeed without authentication
curl http://localhost:8080/health
```

### 2. Test Protected Endpoint Without Authentication

```bash
# This should fail with 401 Unauthorized or 403 Forbidden
curl http://localhost:8080/
```

Expected response: HTTP 401/403

### 3. Test Protected Endpoint With Valid Session

First, you need a valid session token from your STS service. Then:

```bash
# Should succeed and echo back your request
curl -H "Cookie: session-id=YOUR_VALID_SESSION_TOKEN" http://localhost:8080/
```

Expected response: Echoserver output showing your request details with the JWT in headers.

### 4. Test Via Istio Gateway (External Access)

Access echoserver through the Istio ingress gateway on the `/echo` path:

```bash
# Get the gateway external IP/port
kubectl get svc -n istio-system istio-ingressgateway

# Test public health endpoint via gateway
curl http://localhost/echo/health

# Test protected endpoint via gateway (should fail)
curl http://localhost/echo/

# Test with valid session cookie via gateway
curl -H "Cookie: session-id=YOUR_SESSION" http://localhost/echo/
```

### 5. Test Using Authorization Service CLI

Use the authorization service's built-in test command:

```bash
# From the authorization-service directory
export STS_ADDRESS="sts-service.default.svc.cluster.local:9091"
export NATS_ENABLED="false"  # Disable NATS if not needed for testing

./bin/app check \
  --cookie "session-id=YOUR_SESSION_TOKEN" \
  --path "/" \
  --method "GET" \
  --host "echoserver.default.svc.cluster.local"
```

This will show you the authorization decision without actually calling the echoserver.

## How It Works (Ambient Mode)

1. **Request arrives** at the echoserver service
2. **ztunnel intercepts** the request (L4 proxy in ambient mesh)
3. **Waypoint proxy handles L7** processing and AuthorizationPolicy
4. **Istio AuthorizationPolicy** intercepts the request
5. **External authorization check** is triggered for non-health paths
6. **Waypoint proxy calls** the authorization-service gRPC ExtAuthz API
7. **Authorization service**:
   - Extracts `session-id` cookie from request headers
   - Calls STS `ExchangeSession` to exchange cookie for JWT
   - If successful, returns ALLOW with `Authorization: Bearer <jwt>` header
   - If failed, returns DENY with 401 status
8. **Echoserver receives** the authorized request with JWT in headers

## Cleanup

Remove the echoserver deployment:

```bash
kubectl delete -k k8s/echoserver/
```

## Troubleshooting

### Authorization service not reachable

Check if the authorization service is running:
```bash
kubectl get pods -l app=authorization-service
kubectl logs -l app=authorization-service
```

### External authz provider not configured

Ensure the ext-authz provider is configured in Istio:
```bash
kubectl get configmap ext-authz-provider -n istio-system
```

### STS service not available

Verify STS is running and accessible:
```bash
kubectl get svc sts-service
kubectl get pods -l app=sts
```

### Check waypoint proxy logs (ambient mode)

View the waypoint proxy logs to see authorization decisions:
```bash
# Find the waypoint proxy pod
kubectl get pods -n default -l gateway.istio.io/managed=istio.io-mesh-controller

# View logs
kubectl logs -n default <waypoint-pod-name>
```

## Next Steps

- Configure OpenFGA for fine-grained permission checks
- Add more complex authorization rules (role-based, attribute-based)
- Set up monitoring and observability for authorization metrics
- Integrate with your identity provider for session management
