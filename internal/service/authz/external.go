// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package authz

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyAuth "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	envoyType "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/openfga/go-sdk/client"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	stsv1 "github.com/canonical/authorization-service/client/v1/sts"
	"github.com/canonical/authorization-service/internal/integration/openfga"
	"github.com/canonical/authorization-service/internal/logging"
	"github.com/canonical/authorization-service/internal/service/rules"
)

const (
	sessionCookieName             = "session_id"
	sessionCookieNamePrefix       = sessionCookieName + "="
	sessionCookieNamePrefixLength = len(sessionCookieNamePrefix)
)

type ExternalAuthzServiceInterface interface {
	Register(grpcServer *grpc.Server)
	Check(ctx context.Context, req *envoyAuth.CheckRequest) (*envoyAuth.CheckResponse, error)
}

type oidcVerifier interface {
	Verify(ctx context.Context, token string) (*oidc.IDToken, error)
}

// Compile-time check to ensure ExternalAuthzService implements ExternalAuthzServiceInterface
var _ ExternalAuthzServiceInterface = (*ExternalAuthzService)(nil)

type ExternalAuthzService struct {
	envoyAuth.UnimplementedAuthorizationServer

	verifier            oidcVerifier
	sts                 stsv1.SecurityTokenServiceClient
	resourceMapper      rules.ResourceMapperInterface
	fga                 openfga.OpenFGAClientInterface
	multitenancyEnabled bool
	metrics             Metrics

	logger *slog.Logger
	tracer trace.Tracer
}

// NewExternalAuthzService constructs an ExternalAuthzService. If metrics is
// nil a no-op is used.
func NewExternalAuthzService(
	verifier oidcVerifier,
	sts stsv1.SecurityTokenServiceClient,
	resourceMapper rules.ResourceMapperInterface,
	fga openfga.OpenFGAClientInterface,
	multitenancyEnabled bool,
	metrics Metrics,
	logger *slog.Logger,
	tracer trace.Tracer,
) *ExternalAuthzService {
	if metrics == nil {
		metrics = NoopMetrics{}
	}
	return &ExternalAuthzService{
		verifier:            verifier,
		sts:                 sts,
		resourceMapper:      resourceMapper,
		fga:                 fga,
		multitenancyEnabled: multitenancyEnabled,
		metrics:             metrics,
		logger:              logger,
		tracer:              tracer,
	}
}

// Register registers the service with the gRPC server
func (s *ExternalAuthzService) Register(grpcServer *grpc.Server) {
	envoyAuth.RegisterAuthorizationServer(grpcServer, s)
}

func (s *ExternalAuthzService) Check(ctx context.Context, req *envoyAuth.CheckRequest) (*envoyAuth.CheckResponse, error) {
	ctx, span := s.tracer.Start(ctx, "authz.ExternalAuthzService.Check")
	defer span.End()

	httpReq := req.GetAttributes().GetRequest().GetHttp()
	method := httpReq.GetMethod()
	path := httpReq.GetPath()
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	start := time.Now()
	resp, outcome, err := s.check(ctx, req)
	duration := time.Since(start)
	s.metrics.RecordCheck(outcome.result, outcome.reason, duration)

	args := []any{
		"request_id", logging.RequestIDFromContext(ctx),
		"result", outcome.result,
		"reason", outcome.reason,
		"method", method,
		"path", path,
		"subject", outcome.subject,
		"tenant", outcome.tenant,
		"duration_ms", duration.Milliseconds(),
	}
	if err != nil {
		args = append(args, "error", err)
	}
	s.logger.InfoContext(ctx, "Authorization decision", args...)

	return resp, err
}

// checkOutcome carries the result of check() beyond the envoy
// response/error pair: result/reason (used for metrics, at fixed
// cardinality) and subject/tenant (used for the "Authorization decision"
// log line emitted once by Check).
type checkOutcome struct {
	result  string
	reason  string
	subject string
	tenant  string
}

// check performs the actual authorization decision.
func (s *ExternalAuthzService) check(ctx context.Context, req *envoyAuth.CheckRequest) (*envoyAuth.CheckResponse, checkOutcome, error) {
	httpReq := req.GetAttributes().GetRequest().GetHttp()
	method := httpReq.GetMethod()
	path := httpReq.GetPath()

	// Strip query string if present.
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	s.logger.Debug("Check request received", "method", method, "path", path)

	headers := httpReq.GetHeaders()
	cookies, ok := headers["cookie"]
	if !ok {
		s.logger.Debug("No cookie header found in request")
		return unauthorized("no session cookie provided"), checkOutcome{result: "deny", reason: "no_cookie"}, nil
	}

	sessionValue := extractSessionCookie(cookies)
	if sessionValue == "" {
		s.logger.Debug("Session cookie not found in cookie header")
		return unauthorized("session cookie not found"), checkOutcome{result: "deny", reason: "no_session"}, nil
	}

	stsStart := time.Now()
	exchangeResp, err := s.sts.ExchangeSession(ctx, &stsv1.ExchangeRequest{
		SessionCookie: sessionValue,
	})
	s.metrics.ObserveSTSExchange(time.Since(stsStart))

	if err != nil {
		return forbidden(err.Error()), checkOutcome{result: "deny", reason: "sts_exchange_failed"}, nil
	}

	claims, err := s.extractJWTClaims(ctx, exchangeResp.GetAccessToken())
	if err != nil || claims.Sub == "" {
		s.logger.Debug("Failed to extract claims from JWT", "error", err)
		// If there is no explicit subject, we can't perform an authz check.
		return unauthorized("issue with STS JWT"), checkOutcome{result: "deny", reason: "jwt_invalid"}, nil
	}
	userIdentity := claims.Sub

	tenant := ""
	if s.multitenancyEnabled {
		tenant = claims.Org
	}

	mapStart := time.Now()
	tuples, matchedRule, err := s.resourceMapper.Map(ctx, userIdentity, method, path)
	s.metrics.ObserveResourceMap(time.Since(mapStart))
	if err != nil {
		s.logger.Error("Failed to map resource", "method", method, "path", path, "error", err)
		return forbidden("internal error during authorization"), checkOutcome{result: "error", reason: "internal_error", subject: userIdentity, tenant: tenant}, nil
	}

	// No matching rule found — forbidden (if istio invoked the extAuthz then rules must be present, or something is wrong)
	if tuples == nil || len(tuples) == 0 {
		s.logger.Debug("No authorization rule matched", "method", method, "path", path)
		return forbidden(fmt.Sprintf("No authorization rule matched method %s path %s", method, path)), checkOutcome{result: "deny", reason: "no_rule_matched", subject: userIdentity, tenant: tenant}, nil
	}

	// if no matched rule then return an error, if it's going through Cerberus then it needs a rule.
	// Public endpoints must be ALLOWed via AuthorizationPolicy
	if matchedRule == nil {
		s.logger.Error("No matching rule was returned found")
		return nil, checkOutcome{result: "error", reason: "internal_error", subject: userIdentity, tenant: tenant}, fmt.Errorf("no matching rule found")
	}

	if s.multitenancyEnabled {
		if matchedRule.Tenant == nil || *matchedRule.Tenant == "" {
			s.logger.Error("Rule does not have an associated tenant, but multitenancy is enabled", "rule_id", matchedRule.Id)
			return nil, checkOutcome{result: "error", reason: "internal_error", subject: userIdentity, tenant: tenant}, fmt.Errorf("rule %s has no tenant but multitenancy is enabled", matchedRule.Id)
		}
	}

	// Always populate check context for each tuple
	for i := range tuples {
		checkCtx := map[string]interface{}{
			"tenant_enabled": s.multitenancyEnabled,
			"user_tenant":    claims.Org,
		}
		tuples[i].Context = &checkCtx
	}

	// [4] Query OpenFGA.
	fgaStart := time.Now()
	batchCheckResp, err := s.fga.
		BatchCheck(ctx).
		Body(client.ClientBatchCheckRequest{
			Checks: tuples,
		}).
		Execute()
	s.metrics.ObserveOpenFGACheck(time.Since(fgaStart))
	if err != nil {
		s.logger.Error("OpenFGA check failed", "error", err)
		return forbidden("authorization check failed"), checkOutcome{result: "error", reason: "openfga_error", subject: userIdentity, tenant: tenant}, nil
	}

	results, ok := batchCheckResp.GetResultOk()
	if !ok {
		s.logger.Error("OpenFGA check failed, batch check result is not OK")
		return forbidden("access denied"), checkOutcome{result: "error", reason: "openfga_error", subject: userIdentity, tenant: tenant}, nil
	}

	for _, result := range *results {
		if result.HasError() {
			s.logger.Error("OpenFGA check failed", "error", result.GetError())
			return forbidden(fmt.Sprintf("access denied for user %s", userIdentity)), checkOutcome{result: "error", reason: "openfga_error", subject: userIdentity, tenant: tenant}, nil
		}

		allowed, ok := result.GetAllowedOk()
		if !ok {
			s.logger.Debug("Access denied for user", "user", userIdentity, "error", result.GetError())
			return forbidden(fmt.Sprintf("access denied for user %s", userIdentity)), checkOutcome{result: "error", reason: "openfga_error", subject: userIdentity, tenant: tenant}, nil
		}

		if !*allowed {
			s.logger.Debug("Access denied for user", "user", userIdentity)
			return forbidden(fmt.Sprintf("access denied for user %s", userIdentity)), checkOutcome{result: "deny", reason: "openfga_denied", subject: userIdentity, tenant: tenant}, nil
		}
	}

	return okResponse(exchangeResp.AccessToken), checkOutcome{result: "allow", reason: "ok", subject: userIdentity, tenant: tenant}, nil
}

type tokenClaims struct {
	Sub string `json:"sub"`
	Org string `json:"org"`
}

// extractJWTClaims validates the JWT using the OIDC key set and returns the claims
func (s *ExternalAuthzService) extractJWTClaims(ctx context.Context, token string) (*tokenClaims, error) {
	// Verify and parse the token
	idToken, err := s.verifier.Verify(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("failed to verify token: %w", err)
	}

	var claims tokenClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to extract claims: %w", err)
	}

	if claims.Sub == "" {
		return nil, fmt.Errorf("token does not contain a 'sub' claim")
	}

	return &claims, nil
}

// okResponse builds a successful CheckResponse that forwards the JWT as a bearer token.
func okResponse(accessToken string) *envoyAuth.CheckResponse {
	return &envoyAuth.CheckResponse{
		Status: &status.Status{
			Code: int32(codes.OK),
		},
		HttpResponse: &envoyAuth.CheckResponse_OkResponse{
			OkResponse: &envoyAuth.OkHttpResponse{
				Headers: []*corev3.HeaderValueOption{
					{
						Header: &corev3.HeaderValue{
							Key:   "authorization",
							Value: fmt.Sprintf("Bearer %s", accessToken),
						},
					},
				},
			},
		},
	}
}

// extractSessionCookie parses the Cookie header string and extracts the "session" cookie value
func extractSessionCookie(cookieHeader string) string {
	// Cookie header format: "cookie1=value1; cookie2=value2; session=sessionvalue"
	cookies := splitCookies(cookieHeader)
	for _, cookie := range cookies {
		if len(cookie) >= sessionCookieNamePrefixLength && cookie[:sessionCookieNamePrefixLength] == sessionCookieNamePrefix {
			return cookie[sessionCookieNamePrefixLength:]
		}
	}

	return ""
}

// splitCookies splits a cookie header into individual cookie strings
func splitCookies(cookieHeader string) []string {
	if cookieHeader == "" {
		return []string{}
	}

	var cookies []string
	start := 0

	for i := 0; i < len(cookieHeader); i++ {
		if cookieHeader[i] == ';' {
			trimmed := strings.TrimSpace(cookieHeader[start:i])
			if trimmed != "" {
				cookies = append(cookies, trimmed)
			}
			start = i + 1
		}
	}

	if start < len(cookieHeader) {
		trimmed := strings.TrimSpace(cookieHeader[start:])
		if trimmed != "" {
			cookies = append(cookies, trimmed)
		}
	}

	return cookies
}

func unauthorized(body string) *envoyAuth.CheckResponse {
	return denyResponse(body, envoyType.StatusCode_Unauthorized)
}

func forbidden(body string) *envoyAuth.CheckResponse {
	return denyResponse(body, envoyType.StatusCode_Forbidden)
}

func denyResponse(body string, code envoyType.StatusCode) *envoyAuth.CheckResponse {
	var statusCode codes.Code
	switch code {
	case envoyType.StatusCode_Unauthorized:
		statusCode = codes.Unauthenticated
	case envoyType.StatusCode_Forbidden:
		statusCode = codes.PermissionDenied
	}

	return &envoyAuth.CheckResponse{
		Status: &status.Status{
			Code: int32(statusCode),
		},
		HttpResponse: &envoyAuth.CheckResponse_DeniedResponse{
			DeniedResponse: &envoyAuth.DeniedHttpResponse{
				Status: &envoyType.HttpStatus{
					Code: code,
				},
				Body: body,
			},
		},
	}
}
