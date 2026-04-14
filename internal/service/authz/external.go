package authz

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

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
	"github.com/canonical/authorization-service/internal/service/rules"
)

const (
	sessionCookieName             = "session"
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

	verifier       oidcVerifier
	sts            stsv1.SecurityTokenServiceClient
	resourceMapper rules.ResourceMapperInterface
	fga            openfga.OpenFGAClientInterface

	logger *slog.Logger
	tracer trace.Tracer
}

func NewExternalAuthzService(
	verifier oidcVerifier,
	sts stsv1.SecurityTokenServiceClient,
	resourceMapper rules.ResourceMapperInterface,
	fga openfga.OpenFGAClientInterface,
	logger *slog.Logger,
	tracer trace.Tracer,
) *ExternalAuthzService {
	return &ExternalAuthzService{
		verifier:       verifier,
		sts:            sts,
		resourceMapper: resourceMapper,
		fga:            fga,
		logger:         logger,
		tracer:         tracer,
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

	// Strip query string if present.
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	s.logger.Debug("Check request received", "method", method, "path", path)

	headers := httpReq.GetHeaders()
	cookies, ok := headers["cookie"]
	if !ok {
		s.logger.Debug("No cookie header found in request")
		return unauthorized("No session cookie provided"), nil
	}

	sessionValue := extractSessionCookie(cookies)
	if sessionValue == "" {
		s.logger.Debug("Session cookie not found in cookie header")
		return unauthorized("Session cookie not found"), nil
	}

	exchangeResp, err := s.sts.ExchangeSession(ctx, &stsv1.ExchangeRequest{
		SessionCookie: sessionValue,
	})

	if err != nil {
		s.logger.Debug("Failed to exchange session cookie", "error", err)
		return forbidden(err.Error()), nil
	}

	userIdentity, err := s.extractJWTSubject(ctx, exchangeResp.GetAccessToken())
	if err != nil || userIdentity == "" {
		s.logger.Debug("Could not extract subject from JWT, skipping authorization check", "error", err)
		// If there is no explicit subject, we can't perform an authz check.
		return okResponse(exchangeResp.AccessToken), nil
	}

	tuples, err := s.resourceMapper.Map(ctx, userIdentity, method, path)
	if err != nil {
		s.logger.Error("Failed to map resource", "method", method, "path", path, "error", err)
		return forbidden("internal error during authorization"), nil
	}

	// No matching rule found — forbidden (if istio invoked the extAuthz then rules must be present, or something is wrong)
	if tuples == nil || len(tuples) == 0 {
		s.logger.Debug("No authorization rule matched", "method", method, "path", path)
		return forbidden(fmt.Sprintf("No authorization rule matched method %s path %s", method, path)), nil
	}

	// [4] Query OpenFGA.
	batchCheckResp, err := s.fga.
		BatchCheck(ctx).
		Body(client.ClientBatchCheckRequest{
			Checks: tuples,
		}).
		Execute()
	if err != nil {
		s.logger.Error("OpenFGA check failed", "error", err)
		return forbidden("authorization check failed"), nil
	}

	results, ok := batchCheckResp.GetResultOk()
	if !ok {
		s.logger.Error("OpenFGA check failed, batch check result is not OK")
		return forbidden("access denied"), nil
	}

	for _, result := range *results {
		if result.HasError() {
			s.logger.Error("OpenFGA check failed", "error", result.GetError())
			return forbidden(fmt.Sprintf("access denied for user %s", userIdentity)), nil
		}

		allowed, ok := result.GetAllowedOk()
		if !ok {
			s.logger.Debug("Access denied for user", "user", userIdentity, "error", result.GetError())
			return forbidden(fmt.Sprintf("access denied for user %s", userIdentity)), nil
		}

		if !*allowed {
			s.logger.Debug("Access denied for user", "user", userIdentity)
			return forbidden(fmt.Sprintf("access denied for user %s", userIdentity)), nil
		}
	}

	return okResponse(exchangeResp.AccessToken), nil
}

// extractJWTSubject validates the JWT using the OIDC key set and returns the "sub" claim
func (s *ExternalAuthzService) extractJWTSubject(ctx context.Context, token string) (string, error) {
	// Verify and parse the token
	idToken, err := s.verifier.Verify(ctx, token)
	if err != nil {
		return "", fmt.Errorf("failed to verify token: %w", err)
	}

	var claims struct {
		Sub string `json:"sub"`
	}

	if err := idToken.Claims(&claims); err != nil {
		return "", fmt.Errorf("failed to extract claims: %w", err)
	}

	if claims.Sub == "" {
		return "", fmt.Errorf("token does not contain a 'sub' claim")
	}

	return claims.Sub, nil
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
