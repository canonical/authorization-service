package authz

import (
    "context"
    "log/slog"

    envoyCore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
    envoyAuth "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
    envoyType "github.com/envoyproxy/go-control-plane/envoy/type/v3"
    "go.opentelemetry.io/otel/trace"
    "google.golang.org/genproto/googleapis/rpc/status"
    "google.golang.org/grpc"
    "google.golang.org/grpc/codes"
)

type ExternalAuthzServiceInterface interface {
    Register(grpcServer *grpc.Server)
    Check(ctx context.Context, req *envoyAuth.CheckRequest) (*envoyAuth.CheckResponse, error)
}

// Compile-time check to ensure ExternalAuthzService implements ExternalAuthzServiceInterface
var _ ExternalAuthzServiceInterface = (*ExternalAuthzService)(nil)

type ExternalAuthzService struct {
    envoyAuth.UnimplementedAuthorizationServer

    logger *slog.Logger
    tracer trace.Tracer
}

func NewExternalAuthzService(logger *slog.Logger, tracer trace.Tracer) *ExternalAuthzService {
    return &ExternalAuthzService{logger: logger, tracer: tracer}
}

// Register registers the service with the gRPC server
func (s *ExternalAuthzService) Register(grpcServer *grpc.Server) {
    envoyAuth.RegisterAuthorizationServer(grpcServer, s)
}

func (s *ExternalAuthzService) Check(ctx context.Context, req *envoyAuth.CheckRequest) (*envoyAuth.CheckResponse, error) {
    ctx, span := s.tracer.Start(ctx, "authz.ExternalAuthzService.Check")
    defer span.End()

    headers := req.GetAttributes().GetRequest().GetHttp().GetHeaders()
    authHeader := headers["authorization"]

    if !isAuthorized(authHeader) {
        return denyResponse("Unauthorized"), nil
    }

    return &envoyAuth.CheckResponse{
        Status: &status.Status{
            Code: int32(codes.OK),
        },
        HttpResponse: &envoyAuth.CheckResponse_OkResponse{
            OkResponse: &envoyAuth.OkHttpResponse{
                Headers: []*envoyCore.HeaderValueOption{
                    {
                        Header: &envoyCore.HeaderValue{
                            Key:   "x-custom-header",
                            Value: "custom-value",
                        },
                    },
                },
                HeadersToRemove:         nil,
                ResponseHeadersToAdd:    nil,
                QueryParametersToSet:    nil,
                QueryParametersToRemove: nil,
            },
        },
    }, nil
}

func denyResponse(body string) *envoyAuth.CheckResponse {
    return &envoyAuth.CheckResponse{
        Status: &status.Status{
            Code: int32(codes.PermissionDenied),
        },
        HttpResponse: &envoyAuth.CheckResponse_DeniedResponse{
            DeniedResponse: &envoyAuth.DeniedHttpResponse{
                Status: &envoyType.HttpStatus{
                    Code: envoyType.StatusCode_Unauthorized,
                },
                Body: body,
                Headers: []*envoyCore.HeaderValueOption{
                    {
                        Header: &envoyCore.HeaderValue{
                            Key:   "x-custom-header",
                            Value: "custom-value",
                        },
                        AppendAction: 0,
                    },
                },
            },
        },
    }
}

func isAuthorized(authHeader string) bool {
    return authHeader == "Bearer valid-token"
}
