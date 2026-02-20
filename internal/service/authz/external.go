package authz

import (
    "context"
    "fmt"
    "log/slog"
    "strings"

    corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
    envoyAuth "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
    envoyType "github.com/envoyproxy/go-control-plane/envoy/type/v3"
    "go.opentelemetry.io/otel/trace"
    "google.golang.org/genproto/googleapis/rpc/status"
    "google.golang.org/grpc"
    "google.golang.org/grpc/codes"

    stsv1 "github.com/canonical/authorization-service/client/v1/sts"
)

const (
    sessionCookieName             = "session-id"
    sessionCookieNamePrefix       = sessionCookieName + "="
    sessionCookieNamePrefixLength = len(sessionCookieNamePrefix)
)

type ExternalAuthzServiceInterface interface {
    Register(grpcServer *grpc.Server)
    Check(ctx context.Context, req *envoyAuth.CheckRequest) (*envoyAuth.CheckResponse, error)
}

// Compile-time check to ensure ExternalAuthzService implements ExternalAuthzServiceInterface
var _ ExternalAuthzServiceInterface = (*ExternalAuthzService)(nil)

type ExternalAuthzService struct {
    envoyAuth.UnimplementedAuthorizationServer

    sts stsv1.SecurityTokenServiceClient

    logger *slog.Logger
    tracer trace.Tracer
}

func NewExternalAuthzService(sts stsv1.SecurityTokenServiceClient, logger *slog.Logger, tracer trace.Tracer) *ExternalAuthzService {
    return &ExternalAuthzService{
        sts:    sts,
        logger: logger,
        tracer: tracer,
    }
}

// Register registers the service with the gRPC server
func (s *ExternalAuthzService) Register(grpcServer *grpc.Server) {
    envoyAuth.RegisterAuthorizationServer(grpcServer, s)
}

func (s *ExternalAuthzService) Check(ctx context.Context, req *envoyAuth.CheckRequest) (*envoyAuth.CheckResponse, error) {
    ctx, span := s.tracer.Start(ctx, "authz.ExternalAuthzService.Check")
    defer span.End()

    // Extract the session cookie from the request headers
    headers := req.GetAttributes().GetRequest().GetHttp().GetHeaders()
    sessionCookie, ok := headers["cookie"]
    if !ok {
        s.logger.Debug("No cookie header found in request")
        return denyResponse("No session cookie provided"), nil
    }

    // Parse the cookie header to extract the "session" cookie value
    sessionValue := extractSessionCookie(sessionCookie)
    if sessionValue == "" {
        s.logger.Debug("Session cookie not found in cookie header")
        return denyResponse("Session cookie not found"), nil
    }

    // Exchange the session cookie for a JWT token
    exchangeResp, err := s.sts.ExchangeSession(ctx, &stsv1.ExchangeRequest{
        SessionCookie: sessionValue,
    })

    if err != nil {
        s.logger.Debug("Failed to exchange session-id cookie", "error", err)
        return denyResponse(err.Error()), nil
    }

    // Return successful response with JWT as bearer token in ResponseHeadersToAdd
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
                            Value: fmt.Sprintf("Bearer %s", exchangeResp.AccessToken),
                        },
                    },
                },
                HeadersToRemove: nil,
            },
        },
    }, nil
}

// extractSessionCookie parses the Cookie header string and extracts the "session-id" cookie value
func extractSessionCookie(cookieHeader string) string {
    // Cookie header format: "cookie1=value1; cookie2=value2; session-id=sessionvalue"
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

func denyResponse(body string) *envoyAuth.CheckResponse {
    return &envoyAuth.CheckResponse{
        Status: &status.Status{
            Code: int32(codes.Unauthenticated),
        },
        HttpResponse: &envoyAuth.CheckResponse_DeniedResponse{
            DeniedResponse: &envoyAuth.DeniedHttpResponse{
                Status: &envoyType.HttpStatus{
                    Code: envoyType.StatusCode_Unauthorized,
                },
                Body: body,
            },
        },
    }
}
