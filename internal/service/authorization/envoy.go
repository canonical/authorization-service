// Copyright 2025 Canonical Ltd
// SPDX-License-Identifier: AGPL-3.0

package authorization

import (
	"context"
	"log/slog"
	"strings"

	stsv1 "github.com/canonical/authorization-service/api/proto/v1"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
)

// EnvoyAuthzService implements Envoy's external authorization API v3
type EnvoyAuthzService struct {
	authv3.UnimplementedAuthorizationServer
	stsClient stsv1.SecurityTokenServiceClient
	logger    *slog.Logger
}

// NewEnvoyAuthzService creates a new Envoy authorization service
func NewEnvoyAuthzService(stsClient stsv1.SecurityTokenServiceClient, logger *slog.Logger) *EnvoyAuthzService {
	return &EnvoyAuthzService{
		stsClient: stsClient,
		logger:    logger,
	}
}

// Check performs the authorization check for Envoy
func (s *EnvoyAuthzService) Check(ctx context.Context, req *authv3.CheckRequest) (*authv3.CheckResponse, error) {
	// Extract request attributes
	attrs := req.GetAttributes()
	if attrs == nil || attrs.GetRequest() == nil || attrs.GetRequest().GetHttp() == nil {
		s.logger.Error("missing request attributes")
		return s.deny(req, "missing request attributes"), nil
	}

	httpReq := attrs.GetRequest().GetHttp()

	s.logger.Info("envoy authorization check",
		"method", httpReq.GetMethod(),
		"path", httpReq.GetPath(),
		"host", httpReq.GetHost(),
	)

	// Extract session cookie from headers
	sessionCookie := s.extractSessionCookie(httpReq.GetHeaders())
	if sessionCookie == "" {
		s.logger.Warn("missing session cookie", "path", httpReq.GetPath())
		return s.denyUnauthorized(req, "missing session cookie"), nil
	}

	// Exchange session cookie for JWT via STS
	exchangeResp, err := s.stsClient.ExchangeSession(ctx, &stsv1.ExchangeRequest{
		SessionCookie: sessionCookie,
	})
	if err != nil {
		s.logger.Error("failed to exchange session cookie",
			"error", err,
			"path", httpReq.GetPath(),
		)
		return s.denyUnauthorized(req, "invalid or expired session"), nil
	}

	if exchangeResp.GetAccessToken() == "" {
		s.logger.Error("STS returned empty access token", "path", httpReq.GetPath())
		return s.denyUnauthorized(req, "authentication failed"), nil
	}

	// Authorization successful - forward JWT to upstream
	s.logger.Info("authorization granted",
		"path", httpReq.GetPath(),
		"method", httpReq.GetMethod(),
	)

	return s.allowWithJWT(req, exchangeResp.GetAccessToken()), nil
}

// extractSessionCookie extracts the session-id cookie from request headers
func (s *EnvoyAuthzService) extractSessionCookie(headers map[string]string) string {
	cookieHeader, ok := headers["cookie"]
	if !ok {
		return ""
	}

	// Parse cookies (format: "name1=value1; name2=value2")
	cookies := strings.Split(cookieHeader, ";")
	for _, cookie := range cookies {
		cookie = strings.TrimSpace(cookie)
		parts := strings.SplitN(cookie, "=", 2)
		if len(parts) == 2 && parts[0] == "session-id" {
			return parts[1]
		}
	}

	return ""
}

// allow creates a successful authorization response
func (s *EnvoyAuthzService) allow(req *authv3.CheckRequest, reason string) *authv3.CheckResponse {
	return &authv3.CheckResponse{
		Status: &status.Status{
			Code:    int32(codes.OK),
			Message: reason,
		},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{},
		},
	}
}

// allowWithJWT creates a successful response with JWT in Authorization header
func (s *EnvoyAuthzService) allowWithJWT(req *authv3.CheckRequest, accessToken string) *authv3.CheckResponse {
	return &authv3.CheckResponse{
		Status: &status.Status{
			Code:    int32(codes.OK),
			Message: "authorized",
		},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{
				Headers: []*corev3.HeaderValueOption{
					{
						Header: &corev3.HeaderValue{
							Key:   "authorization",
							Value: "Bearer " + accessToken,
						},
					},
				},
			},
		},
	}
}

// deny creates a denial authorization response (403 Forbidden)
func (s *EnvoyAuthzService) deny(req *authv3.CheckRequest, reason string) *authv3.CheckResponse {
	return &authv3.CheckResponse{
		Status: &status.Status{
			Code:    int32(code.Code_PERMISSION_DENIED),
			Message: reason,
		},
		HttpResponse: &authv3.CheckResponse_DeniedResponse{
			DeniedResponse: &authv3.DeniedHttpResponse{
				Status: &typev3.HttpStatus{
					Code: typev3.StatusCode_Forbidden,
				},
				Body: reason,
			},
		},
	}
}

// denyUnauthorized creates an unauthorized response (401)
func (s *EnvoyAuthzService) denyUnauthorized(req *authv3.CheckRequest, reason string) *authv3.CheckResponse {
	return &authv3.CheckResponse{
		Status: &status.Status{
			Code:    int32(code.Code_UNAUTHENTICATED),
			Message: reason,
		},
		HttpResponse: &authv3.CheckResponse_DeniedResponse{
			DeniedResponse: &authv3.DeniedHttpResponse{
				Status: &typev3.HttpStatus{
					Code: typev3.StatusCode_Unauthorized,
				},
				Body: reason,
			},
		},
	}
}
