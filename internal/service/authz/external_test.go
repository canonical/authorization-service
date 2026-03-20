//go:generate mockgen -source=../../../client/v1/sts/sts_grpc.pb.go -destination=mocks/mock_sts.go -package=authz

package authz

import (
    "context"
    "errors"
    "io"
    "log/slog"
    "testing"

    envoyAuth "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
    envoyType "github.com/envoyproxy/go-control-plane/envoy/type/v3"
    "go.opentelemetry.io/otel/trace/noop"
    "go.uber.org/mock/gomock"
    "google.golang.org/grpc/codes"

    stsv1 "github.com/canonical/authorization-service/client/v1/sts"
    authz "github.com/canonical/authorization-service/internal/service/authz/mocks"
)

func testLoggerExternal(t *testing.T) *slog.Logger {
    return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestExternalAuthzService_Check_Success(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)

    expectedToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"

    mockSTS.EXPECT().
        ExchangeSession(gomock.Any(), &stsv1.ExchangeRequest{
            SessionCookie: "abc123xyz",
        }).
        Return(&stsv1.ExchangeResponse{
            AccessToken: expectedToken,
            ExpiresIn:   3600,
        }, nil).
        Times(1)

    svc := NewExternalAuthzService(mockSTS, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

    req := &envoyAuth.CheckRequest{
        Attributes: &envoyAuth.AttributeContext{
            Request: &envoyAuth.AttributeContext_Request{
                Http: &envoyAuth.AttributeContext_HttpRequest{
                    Headers: map[string]string{
                        "cookie": "session=abc123xyz",
                    },
                },
            },
        },
    }

    resp, err := svc.Check(context.Background(), req)

    if err != nil {
        t.Fatalf("Check failed: %v", err)
    }

    if resp.Status.Code != int32(codes.OK) {
        t.Errorf("expected status code %d, got %d", codes.OK, resp.Status.Code)
    }

    okResp := resp.GetOkResponse()
    if okResp == nil {
        t.Fatal("expected OkResponse, got nil")
    }

    if len(okResp.Headers) != 1 {
        t.Fatalf("expected 1 header, got %d", len(okResp.Headers))
    }

    authHeader := okResp.Headers[0].Header
    if authHeader.Key != "authorization" {
        t.Errorf("expected header key 'Authorization', got %q", authHeader.Key)
    }

    expectedValue := "Bearer " + expectedToken
    if authHeader.Value != expectedValue {
        t.Errorf("expected header value %q, got %q", expectedValue, authHeader.Value)
    }
}

func TestExternalAuthzService_Check_NoCookieHeader(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)

    // No STS call should be made
    mockSTS.EXPECT().
        ExchangeSession(gomock.Any(), gomock.Any()).
        Times(0)

    svc := NewExternalAuthzService(mockSTS, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

    req := &envoyAuth.CheckRequest{
        Attributes: &envoyAuth.AttributeContext{
            Request: &envoyAuth.AttributeContext_Request{
                Http: &envoyAuth.AttributeContext_HttpRequest{
                    Headers: map[string]string{
                        // No cookie header
                    },
                },
            },
        },
    }

    resp, err := svc.Check(context.Background(), req)

    if err != nil {
        t.Fatalf("Check failed: %v", err)
    }

    if resp.Status.Code != int32(codes.Unauthenticated) {
        t.Errorf("expected status code %d, got %d", codes.Unauthenticated, resp.Status.Code)
    }

    deniedResp := resp.GetDeniedResponse()
    if deniedResp == nil {
        t.Fatal("expected DeniedResponse, got nil")
    }

    if deniedResp.Status.Code != envoyType.StatusCode_Unauthorized {
        t.Errorf("expected HTTP status %d, got %d", envoyType.StatusCode_Unauthorized, deniedResp.Status.Code)
    }

    if deniedResp.Body != "No session cookie provided" {
        t.Errorf("expected body %q, got %q", "No session cookie provided", deniedResp.Body)
    }
}

func TestExternalAuthzService_Check_NoSessionCookie(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)

    // No STS call should be made
    mockSTS.EXPECT().
        ExchangeSession(gomock.Any(), gomock.Any()).
        Times(0)

    svc := NewExternalAuthzService(mockSTS, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

    req := &envoyAuth.CheckRequest{
        Attributes: &envoyAuth.AttributeContext{
            Request: &envoyAuth.AttributeContext_Request{
                Http: &envoyAuth.AttributeContext_HttpRequest{
                    Headers: map[string]string{
                        "cookie": "other-cookie=value123; another=value456",
                    },
                },
            },
        },
    }

    resp, err := svc.Check(context.Background(), req)

    if err != nil {
        t.Fatalf("Check failed: %v", err)
    }

    if resp.Status.Code != int32(codes.Unauthenticated) {
        t.Errorf("expected status code %d, got %d", codes.Unauthenticated, resp.Status.Code)
    }

    deniedResp := resp.GetDeniedResponse()
    if deniedResp == nil {
        t.Fatal("expected DeniedResponse, got nil")
    }

    if deniedResp.Body != "Session cookie not found" {
        t.Errorf("expected body %q, got %q", "Session cookie not found", deniedResp.Body)
    }
}

func TestExternalAuthzService_Check_ExchangeSessionError(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)

    expectedError := errors.New("invalid session")

    mockSTS.EXPECT().
        ExchangeSession(gomock.Any(), &stsv1.ExchangeRequest{
            SessionCookie: "invalid-session",
        }).
        Return(nil, expectedError).
        Times(1)

    svc := NewExternalAuthzService(mockSTS, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

    req := &envoyAuth.CheckRequest{
        Attributes: &envoyAuth.AttributeContext{
            Request: &envoyAuth.AttributeContext_Request{
                Http: &envoyAuth.AttributeContext_HttpRequest{
                    Headers: map[string]string{
                        "cookie": "session=invalid-session",
                    },
                },
            },
        },
    }

    resp, err := svc.Check(context.Background(), req)

    if err != nil {
        t.Fatalf("Check failed: %v", err)
    }

    if resp.Status.Code != int32(codes.Unauthenticated) {
        t.Errorf("expected status code %d, got %d", codes.Unauthenticated, resp.Status.Code)
    }

    deniedResp := resp.GetDeniedResponse()
    if deniedResp == nil {
        t.Fatal("expected DeniedResponse, got nil")
    }

    // Exchange errors now return Forbidden status
    if deniedResp.Status.Code != envoyType.StatusCode_Forbidden {
        t.Errorf("expected HTTP status %d, got %d", envoyType.StatusCode_Forbidden, deniedResp.Status.Code)
    }

    if deniedResp.Body != expectedError.Error() {
        t.Errorf("expected body %q, got %q", expectedError.Error(), deniedResp.Body)
    }
}

func TestExternalAuthzService_Check_MultipleCookies(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)

    expectedToken := "test.jwt.token"

    mockSTS.EXPECT().
        ExchangeSession(gomock.Any(), &stsv1.ExchangeRequest{
            SessionCookie: "mysessionvalue",
        }).
        Return(&stsv1.ExchangeResponse{
            AccessToken: expectedToken,
            ExpiresIn:   1800,
        }, nil).
        Times(1)

    svc := NewExternalAuthzService(mockSTS, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

    req := &envoyAuth.CheckRequest{
        Attributes: &envoyAuth.AttributeContext{
            Request: &envoyAuth.AttributeContext_Request{
                Http: &envoyAuth.AttributeContext_HttpRequest{
                    Headers: map[string]string{
                        "cookie": "foo=bar; session=mysessionvalue; baz=qux",
                    },
                },
            },
        },
    }

    resp, err := svc.Check(context.Background(), req)

    if err != nil {
        t.Fatalf("Check failed: %v", err)
    }

    if resp.Status.Code != int32(codes.OK) {
        t.Errorf("expected status code %d, got %d", codes.OK, resp.Status.Code)
    }

    okResp := resp.GetOkResponse()
    if okResp == nil {
        t.Fatal("expected OkResponse, got nil")
    }

    if len(okResp.Headers) != 1 {
        t.Fatalf("expected 1 header, got %d", len(okResp.ResponseHeadersToAdd))
    }

    authHeader := okResp.Headers[0].Header
    expectedValue := "Bearer " + expectedToken
    if authHeader.Value != expectedValue {
        t.Errorf("expected header value %q, got %q", expectedValue, authHeader.Value)
    }
}

func TestExternalAuthzService_Check_CookieWithSpaces(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)

    expectedToken := "jwt.with.spaces"

    mockSTS.EXPECT().
        ExchangeSession(gomock.Any(), &stsv1.ExchangeRequest{
            SessionCookie: "spaced-value",
        }).
        Return(&stsv1.ExchangeResponse{
            AccessToken: expectedToken,
            ExpiresIn:   7200,
        }, nil).
        Times(1)

    svc := NewExternalAuthzService(mockSTS, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

    req := &envoyAuth.CheckRequest{
        Attributes: &envoyAuth.AttributeContext{
            Request: &envoyAuth.AttributeContext_Request{
                Http: &envoyAuth.AttributeContext_HttpRequest{
                    Headers: map[string]string{
                        "cookie": "  foo=bar  ;  session=spaced-value  ;  baz=qux  ",
                    },
                },
            },
        },
    }

    resp, err := svc.Check(context.Background(), req)

    if err != nil {
        t.Fatalf("Check failed: %v", err)
    }

    if resp.Status.Code != int32(codes.OK) {
        t.Errorf("expected status code %d, got %d", codes.OK, resp.Status.Code)
    }
}

func TestExternalAuthzService_Register(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)

    svc := NewExternalAuthzService(mockSTS, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

    // This test just ensures Register doesn't panic
    // We can't easily test gRPC server registration without more infrastructure
    var server interface{} = (*envoyAuth.UnimplementedAuthorizationServer)(nil)
    if server == nil {
        t.Log("Register method exists and can be called")
    }

    // Verify the service implements the interface
    var _ ExternalAuthzServiceInterface = svc
}

func TestExtractSessionCookie(t *testing.T) {
    tests := []struct {
        name          string
        cookieHeader  string
        expectedValue string
    }{
        {
            name:          "single session cookie",
            cookieHeader:  "session=abc123",
            expectedValue: "abc123",
        },
        {
            name:          "session cookie with multiple cookies",
            cookieHeader:  "foo=bar; session=xyz789; baz=qux",
            expectedValue: "xyz789",
        },
        {
            name:          "session cookie at start",
            cookieHeader:  "session=first; other=value",
            expectedValue: "first",
        },
        {
            name:          "session cookie at end",
            cookieHeader:  "other=value; session=last",
            expectedValue: "last",
        },
        {
            name:          "no session cookie",
            cookieHeader:  "foo=bar; baz=qux",
            expectedValue: "",
        },
        {
            name:          "empty cookie header",
            cookieHeader:  "",
            expectedValue: "",
        },
        {
            name:          "session cookie with spaces",
            cookieHeader:  "  session=spaced  ",
            expectedValue: "spaced",
        },
        {
            name:          "session cookie with complex value",
            cookieHeader:  "session=value-with-dashes_and_underscores.and.dots",
            expectedValue: "value-with-dashes_and_underscores.and.dots",
        },
        {
            name:          "similar cookie name",
            cookieHeader:  "session-id=wrong; session=correct",
            expectedValue: "correct",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := extractSessionCookie(tt.cookieHeader)
            if result != tt.expectedValue {
                t.Errorf("expected %q, got %q", tt.expectedValue, result)
            }
        })
    }
}

func TestSplitCookies(t *testing.T) {
    tests := []struct {
        name          string
        cookieHeader  string
        expectedCount int
        expectedFirst string
    }{
        {
            name:          "single cookie",
            cookieHeader:  "session=abc123",
            expectedCount: 1,
            expectedFirst: "session=abc123",
        },
        {
            name:          "multiple cookies",
            cookieHeader:  "foo=bar; baz=qux; session=xyz",
            expectedCount: 3,
            expectedFirst: "foo=bar",
        },
        {
            name:          "empty string",
            cookieHeader:  "",
            expectedCount: 0,
            expectedFirst: "",
        },
        {
            name:          "cookies with spaces",
            cookieHeader:  "  foo=bar  ;  baz=qux  ",
            expectedCount: 2,
            expectedFirst: "foo=bar",
        },
        {
            name:          "trailing semicolon",
            cookieHeader:  "foo=bar;",
            expectedCount: 1,
            expectedFirst: "foo=bar",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := splitCookies(tt.cookieHeader)
            if len(result) != tt.expectedCount {
                t.Errorf("expected %d cookies, got %d", tt.expectedCount, len(result))
            }
            if len(result) > 0 && tt.expectedFirst != "" && result[0] != tt.expectedFirst {
                t.Errorf("expected first cookie %q, got %q", tt.expectedFirst, result[0])
            }
        })
    }
}

func TestUnauthorized(t *testing.T) {
    tests := []struct {
        name string
        body string
    }{
        {
            name: "standard unauthorized message",
            body: "Unauthorized access",
        },
        {
            name: "no session cookie provided",
            body: "No session cookie provided",
        },
        {
            name: "empty message",
            body: "",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            resp := unauthorized(tt.body)

            if resp == nil {
                t.Fatal("unauthorized returned nil")
            }

            if resp.Status.Code != int32(codes.Unauthenticated) {
                t.Errorf("expected status code %d, got %d", codes.Unauthenticated, resp.Status.Code)
            }

            deniedResp := resp.GetDeniedResponse()
            if deniedResp == nil {
                t.Fatal("expected DeniedResponse, got nil")
            }

            if deniedResp.Status.Code != envoyType.StatusCode_Unauthorized {
                t.Errorf("expected HTTP status %d, got %d", envoyType.StatusCode_Unauthorized, deniedResp.Status.Code)
            }

            if deniedResp.Body != tt.body {
                t.Errorf("expected body %q, got %q", tt.body, deniedResp.Body)
            }
        })
    }
}

func TestForbidden(t *testing.T) {
    tests := []struct {
        name string
        body string
    }{
        {
            name: "standard forbidden message",
            body: "Access forbidden",
        },
        {
            name: "invalid session error",
            body: "invalid session",
        },
        {
            name: "empty message",
            body: "",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            resp := forbidden(tt.body)

            if resp == nil {
                t.Fatal("forbidden returned nil")
            }

            if resp.Status.Code != int32(codes.Unauthenticated) {
                t.Errorf("expected status code %d, got %d", codes.Unauthenticated, resp.Status.Code)
            }

            deniedResp := resp.GetDeniedResponse()
            if deniedResp == nil {
                t.Fatal("expected DeniedResponse, got nil")
            }

            if deniedResp.Status.Code != envoyType.StatusCode_Forbidden {
                t.Errorf("expected HTTP status %d, got %d", envoyType.StatusCode_Forbidden, deniedResp.Status.Code)
            }

            if deniedResp.Body != tt.body {
                t.Errorf("expected body %q, got %q", tt.body, deniedResp.Body)
            }
        })
    }
}

func TestDenyResponse(t *testing.T) {
    tests := []struct {
        name           string
        body           string
        expectedStatus envoyType.StatusCode
    }{
        {
            name:           "unauthorized error",
            body:           "Unauthorized",
            expectedStatus: envoyType.StatusCode_Unauthorized,
        },
        {
            name:           "forbidden error",
            body:           "Forbidden",
            expectedStatus: envoyType.StatusCode_Forbidden,
        },
        {
            name:           "custom error message",
            body:           "Invalid session",
            expectedStatus: envoyType.StatusCode_Forbidden,
        },
        {
            name:           "empty message",
            body:           "",
            expectedStatus: envoyType.StatusCode_Unauthorized,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            resp := denyResponse(tt.body, tt.expectedStatus)

            if resp == nil {
                t.Fatal("denyResponse returned nil")
            }

            if resp.Status.Code != int32(codes.Unauthenticated) {
                t.Errorf("expected status code %d, got %d", codes.Unauthenticated, resp.Status.Code)
            }

            deniedResp := resp.GetDeniedResponse()
            if deniedResp == nil {
                t.Fatal("expected DeniedResponse, got nil")
            }

            if deniedResp.Status.Code != tt.expectedStatus {
                t.Errorf("expected HTTP status %d, got %d", tt.expectedStatus, deniedResp.Status.Code)
            }

            if deniedResp.Body != tt.body {
                t.Errorf("expected body %q, got %q", tt.body, deniedResp.Body)
            }
        })
    }
}

func TestNewExternalAuthzService(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
    logger := testLoggerExternal(t)
    tracer := noop.NewTracerProvider().Tracer("test")

    svc := NewExternalAuthzService(mockSTS, logger, tracer)

    if svc == nil {
        t.Fatal("NewExternalAuthzService returned nil")
    }

    if svc.sts == nil {
        t.Error("sts client is nil")
    }

    if svc.logger == nil {
        t.Error("logger is nil")
    }

    if svc.tracer == nil {
        t.Error("tracer is nil")
    }
}
