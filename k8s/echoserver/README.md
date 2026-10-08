# Echoserver with Istio External Authorization (Ambient Mode)

This example demonstrates how to protect an application (echoserver) using Istio's external authorization feature with the authorization-service in **ambient mode** (sidecar-less).

## Architecture

```
User / Machine Request → Istio Ingress Gateway → ztunnel → Waypoint Proxy (L7)
                                                                 ↓
                                                         External Authorization Check
                                                                 ↓
                                                         Authorization Service
                                                       ↙                      ↘
                                        Cookie: session_id                Authorization: Bearer
                                                ↓                                   ↓
                                        STS ExchangeSession              Ory Hydra Verify & STS ExchangeToken
                                                       ↘                      ↙
                                                         OpenFGA Batch Check (user:<id>)
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

The echoserver is exposed on the `/echo` path via the Istio gateway (`authorization-service-gateway`):

- **Gateway:** `authorization-service-gateway` (in istio-system namespace)
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
- **Mutual Exclusivity (Option B)**: The request must present *either* a user session cookie OR an OAuth2 Bearer token, but **never both**:
  - **User session**: `Cookie: session=<token>` (or configured cookie name)
    - Authorization service calls STS `ExchangeSession` to exchange the cookie for a scoped internal JWT
  - **M2M Bearer token**: `Authorization: Bearer <m2m_jwt>`
    - Authorization service verifies the token signature against Hydra JWKS and issuer claims, then calls STS `ExchangeToken` to exchange it for a scoped internal JWT
  - **Conflicting credentials**: If both cookie and Bearer token are provided, the request is immediately rejected with HTTP `400 Bad Request` (`conflicting_credentials`)
  - **No credentials**: If neither is provided, the request is rejected with HTTP `401 Unauthorized` (`no_credentials`)
- Authorization service checks authorization against OpenFGA
- Validated STS JWT is forwarded to echoserver in the `Authorization: Bearer <jwt>` header

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
# This should fail with 401 Unauthorized (no_credentials)
curl http://localhost:8080/
```

Expected response: HTTP 401 Unauthorized

### 3. Test Protected Endpoint With Valid Session Cookie (User Auth)

First, obtain a valid session token from your STS/IDP service. Then:

```bash
# Should succeed and echo back your request
curl -H "Cookie: session=YOUR_VALID_SESSION_TOKEN" http://localhost:8080/
```

Expected response: Echoserver output showing your request details with the forwarded STS JWT in headers.

### 4. Test Protected Endpoint With Valid Bearer Token (M2M Auth)

Obtain an OAuth2 access token from Ory Hydra via client credentials grant (`client_credentials`). Then:

```bash
# Should succeed and echo back your request
curl -H "Authorization: Bearer YOUR_HYDRA_ACCESS_TOKEN" http://localhost:8080/
```

Expected response: Echoserver output showing your request details with the forwarded STS JWT in headers.

### 5. Test Protected Endpoint With Conflicting Credentials (Option B)

If both a session cookie and a Bearer token are sent simultaneously, the service strictly rejects the request:

```bash
# Should fail with 400 Bad Request
curl -H "Cookie: session=YOUR_VALID_SESSION_TOKEN" \
     -H "Authorization: Bearer YOUR_HYDRA_ACCESS_TOKEN" \
     http://localhost:8080/
```

Expected response: HTTP 400 Bad Request (`conflicting_credentials`).

### 6. Test Via Istio Gateway (External Access)

Access echoserver through the Istio ingress gateway on the `/echo` path:

```bash
# Get the gateway external IP/port
kubectl get svc -n istio-system istio-ingressgateway

# Test public health endpoint via gateway
curl http://localhost/echo/health

# Test protected endpoint via gateway without auth (should fail with 401)
curl http://localhost/echo/

# Test with valid session cookie via gateway
curl -H "Cookie: session=YOUR_SESSION" http://localhost/echo/

# Test with valid Bearer token via gateway
curl -H "Authorization: Bearer YOUR_HYDRA_ACCESS_TOKEN" http://localhost/echo/
```

### 7. Test Using Authorization Service CLI

Use the authorization service's built-in test command:

```bash
# From the authorization-service directory
export STS_ADDRESS="sts-service.default.svc.cluster.local:9091"
export EXT_AUTHZ_SERVICE_HYDRA_JWK_SET_URL="http://hydra-admin.default.svc.cluster.local:4445/.well-known/jwks.json"
export EXT_AUTHZ_SERVICE_HYDRA_ISSUER="http://hydra-public.default.svc.cluster.local:4444"

# Check with session cookie
./bin/app check \
  --cookie "session=YOUR_SESSION_TOKEN" \
  --path "/" \
  --method "GET" \
  --host "echoserver.default.svc.cluster.local"

# Check with Bearer token
./bin/app check \
  --header "Authorization: Bearer YOUR_HYDRA_ACCESS_TOKEN" \
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
   - Inspects credentials according to **Option B (Strict Mutual Exclusivity)**:
     - If both `session` cookie and `Authorization: Bearer` token are present: returns DENY with HTTP 400 Bad Request (`conflicting_credentials`).
     - If neither is present: returns DENY with HTTP 401 Unauthorized (`no_credentials`).
     - If only `session` cookie is present: calls STS `ExchangeSession` to exchange cookie for an internal JWT.
     - If only `Authorization: Bearer` is present: validates token signature against Hydra JWKS and checks issuer claim locally, then calls STS `ExchangeToken` to exchange the external token for an internal JWT.
   - Evaluates authorization policy against OpenFGA based on user/client identity (`user:<id>`), target path, and HTTP method.
   - If authorization succeeds, returns ALLOW with `Authorization: Bearer <internal_jwt>` header injected for the upstream service.
   - If authorization fails, returns DENY with 401 or 403 status.
8. **Echoserver receives** the authorized request with internal JWT in headers

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
