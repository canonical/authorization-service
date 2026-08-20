// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

//go:generate mockgen -source=../../../client/v1/sts/sts_grpc.pb.go -destination=mocks/mock_sts.go -package=authz
//go:generate mockgen -source=../rules/interfaces.go -destination=mocks/mock_rules.go -package=authz
//go:generate mockgen -source=../../integration/openfga/interfaces.go -destination=mocks/mock_openfga_client.go -package=authz
//go:generate mockgen -source=./external.go -destination=mocks/mock_provider.go -package=authz oidcProvider

package authz

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/coreos/go-oidc/v3/oidc"
	envoyAuth "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	envoyType "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	stsv1 "github.com/canonical/authorization-service/client/v1/sts"
	"github.com/canonical/authorization-service/internal/logging"
	"github.com/canonical/authorization-service/internal/model/rules"
	authz "github.com/canonical/authorization-service/internal/service/authz/mocks"
	"github.com/canonical/authorization-service/internal/testutil"
)

func testLoggerExternal(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// debugCapturingLogger is like testutil.CapturingLogger but captures Debug
// level too, so tests can assert a removed Debug line no longer appears.
func debugCapturingLogger(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	handler := logging.NewTraceHandler(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return slog.New(handler), &buf
}

// decodeLogLines decodes every JSON log line captured in buf.
func decodeLogLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var entries []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("failed to decode log line %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

// decodeAuthzDecisionLine asserts that exactly one "Authorization decision"
// log line was captured in buf and returns its decoded fields.
func decodeAuthzDecisionLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var found []map[string]any
	for _, entry := range decodeLogLines(t, buf) {
		if entry["msg"] == "Authorization decision" {
			found = append(found, entry)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected exactly 1 \"Authorization decision\" log line, got %d", len(found))
	}
	return found[0]
}

// newTestIDToken creates an *oidc.IDToken with the given claims JSON set via reflection,
// since oidc.IDToken.claims is an unexported field.
func newTestIDToken(t *testing.T, claimsJSON []byte) *oidc.IDToken {
	t.Helper()
	token := &oidc.IDToken{}
	rv := reflect.ValueOf(token).Elem()
	f := rv.FieldByName("claims")
	// Use unsafe to write to an unexported field.
	f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	f.Set(reflect.ValueOf(claimsJSON))
	return token
}

// mockBatchCheckRequest is a simple hand-written mock for client.SdkClientBatchCheckRequestInterface
// that allows controlling what Execute() returns without requiring mockgen.
type mockBatchCheckRequest struct {
	response *openfga.BatchCheckResponse
	err      error
}

func (m *mockBatchCheckRequest) Body(_ client.ClientBatchCheckRequest) client.SdkClientBatchCheckRequestInterface {
	return m
}
func (m *mockBatchCheckRequest) Options(_ client.BatchCheckOptions) client.SdkClientBatchCheckRequestInterface {
	return m
}
func (m *mockBatchCheckRequest) Execute() (*openfga.BatchCheckResponse, error) {
	return m.response, m.err
}
func (m *mockBatchCheckRequest) GetContext() context.Context { return context.Background() }
func (m *mockBatchCheckRequest) GetBody() *client.ClientBatchCheckRequest {
	return nil
}
func (m *mockBatchCheckRequest) GetOptions() *client.BatchCheckOptions { return nil }

// boolPtr returns a pointer to the given bool — useful when constructing BatchCheckSingleResult.
func boolPtr(b bool) *bool { return &b }

// buildCheckRequest builds a minimal envoy CheckRequest with the given headers, method and path.
func buildCheckRequest(headers map[string]string, method, path string) *envoyAuth.CheckRequest {
	return &envoyAuth.CheckRequest{
		Attributes: &envoyAuth.AttributeContext{
			Request: &envoyAuth.AttributeContext_Request{
				Http: &envoyAuth.AttributeContext_HttpRequest{
					Headers: headers,
					Method:  method,
					Path:    path,
				},
			},
		},
	}
}

// checkMetricsCall captures a single RecordCheck invocation.
type checkMetricsCall struct {
	result string
	reason string
}

// fakeCheckMetrics is a hand-rolled Metrics fake that records every call so
// tests can assert both the final RecordCheck outcome and which stages were
// actually observed.
type fakeCheckMetrics struct {
	recordCheckCalls  []checkMetricsCall
	stsExchangeCalls  int
	resourceMapCalls  int
	openFGACheckCalls int
}

func (f *fakeCheckMetrics) RecordCheck(result, reason string, _ time.Duration) {
	f.recordCheckCalls = append(f.recordCheckCalls, checkMetricsCall{result: result, reason: reason})
}
func (f *fakeCheckMetrics) ObserveSTSExchange(_ time.Duration)  { f.stsExchangeCalls++ }
func (f *fakeCheckMetrics) ObserveResourceMap(_ time.Duration)  { f.resourceMapCalls++ }
func (f *fakeCheckMetrics) ObserveOpenFGACheck(_ time.Duration) { f.openFGACheckCalls++ }

// assertRecordCheck asserts that RecordCheck was called exactly once with the
// given result/reason pair.
func assertRecordCheck(t *testing.T, m *fakeCheckMetrics, wantResult, wantReason string) {
	t.Helper()
	if len(m.recordCheckCalls) != 1 {
		t.Fatalf("expected exactly 1 RecordCheck call, got %d: %+v", len(m.recordCheckCalls), m.recordCheckCalls)
	}
	got := m.recordCheckCalls[0]
	if got.result != wantResult || got.reason != wantReason {
		t.Errorf("RecordCheck = (%q, %q), want (%q, %q)", got.result, got.reason, wantResult, wantReason)
	}
}

// assertObserveCalls asserts the exact call counts of the three duration
// observers, verifying instrumentation placement (not just the final label).
func assertObserveCalls(t *testing.T, m *fakeCheckMetrics, wantSTS, wantResourceMap, wantOpenFGA int) {
	t.Helper()
	if m.stsExchangeCalls != wantSTS {
		t.Errorf("ObserveSTSExchange calls = %d, want %d", m.stsExchangeCalls, wantSTS)
	}
	if m.resourceMapCalls != wantResourceMap {
		t.Errorf("ObserveResourceMap calls = %d, want %d", m.resourceMapCalls, wantResourceMap)
	}
	if m.openFGACheckCalls != wantOpenFGA {
		t.Errorf("ObserveOpenFGACheck calls = %d, want %d", m.openFGACheckCalls, wantOpenFGA)
	}
}

// --- Check: early-exit paths --------------------------------------------------

// TestExternalAuthzService_Check_NoSubjectEarlyReturn verifies that when the JWT
// does not contain a "sub" claim (or verification fails), the service returns 401
// Unauthorized — no resource mapping or FGA call is made.
func TestExternalAuthzService_Check_NoSubjectEarlyReturn(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	expectedToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.fQ9GIVLJmrq7sVyxP7l1q0R-u5Qlm1EL5nqPWlV2Ydg"

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), &stsv1.ExchangeRequest{SessionCookie: "abc123xyz"}).
		Return(&stsv1.ExchangeResponse{AccessToken: expectedToken, ExpiresIn: 3600}, nil).
		Times(1)

	// Verify fails — no sub extractable.
	mockVerifier.EXPECT().
		Verify(gomock.Any(), expectedToken).
		Return(nil, fmt.Errorf("token does not contain a 'sub' claim")).
		Times(1)

	// Neither resource mapper nor FGA should be called.
	mockResourceMapper.EXPECT().Map(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	mockOpenFGA.EXPECT().BatchCheck(gomock.Any()).Times(0)

	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

	resp, err := svc.Check(context.Background(), buildCheckRequest(
		map[string]string{"cookie": "session_id=abc123xyz"}, "GET", "/api/resource",
	))

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
	assertRecordCheck(t, metrics, "deny", "jwt_invalid")
	assertObserveCalls(t, metrics, 1, 0, 0)
}

func TestExternalAuthzService_Check_NoCookieHeader(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	mockSTS.EXPECT().ExchangeSession(gomock.Any(), gomock.Any()).Times(0)
	mockVerifier.EXPECT().Verify(gomock.Any(), gomock.Any()).Times(0)

	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

	resp, err := svc.Check(context.Background(), buildCheckRequest(map[string]string{}, "GET", "/api"))
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
	if deniedResp.Body != "no session cookie provided" {
		t.Errorf("expected body %q, got %q", "No session cookie provided", deniedResp.Body)
	}
	assertRecordCheck(t, metrics, "deny", "no_cookie")
	assertObserveCalls(t, metrics, 0, 0, 0)
}

func TestExternalAuthzService_Check_NoSessionCookie(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	mockSTS.EXPECT().ExchangeSession(gomock.Any(), gomock.Any()).Times(0)
	mockVerifier.EXPECT().Verify(gomock.Any(), gomock.Any()).Times(0)

	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

	resp, err := svc.Check(context.Background(), buildCheckRequest(
		map[string]string{"cookie": "other-cookie=value123; another=value456"}, "GET", "/api",
	))
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
	if deniedResp.Body != "session cookie not found" {
		t.Errorf("expected body %q, got %q", "session cookie not found", deniedResp.Body)
	}
	assertRecordCheck(t, metrics, "deny", "no_session")
	assertObserveCalls(t, metrics, 0, 0, 0)
}

func TestExternalAuthzService_Check_ExchangeSessionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	expectedError := errors.New("invalid session")

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), &stsv1.ExchangeRequest{SessionCookie: "invalid-session"}).
		Return(nil, expectedError).
		Times(1)
	mockVerifier.EXPECT().Verify(gomock.Any(), gomock.Any()).Times(0)

	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

	resp, err := svc.Check(context.Background(), buildCheckRequest(
		map[string]string{"cookie": "session_id=invalid-session"}, "GET", "/api",
	))
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if resp.Status.Code != int32(codes.PermissionDenied) {
		t.Errorf("expected status code %d, got %d", codes.PermissionDenied, resp.Status.Code)
	}
	deniedResp := resp.GetDeniedResponse()
	if deniedResp == nil {
		t.Fatal("expected DeniedResponse, got nil")
	}
	if deniedResp.Status.Code != envoyType.StatusCode_Forbidden {
		t.Errorf("expected HTTP status %d, got %d", envoyType.StatusCode_Forbidden, deniedResp.Status.Code)
	}
	if deniedResp.Body != expectedError.Error() {
		t.Errorf("expected body %q, got %q", expectedError.Error(), deniedResp.Body)
	}
	assertRecordCheck(t, metrics, "deny", "sts_exchange_failed")
	assertObserveCalls(t, metrics, 1, 0, 0)
}

// --- Check: resource-mapper paths --------------------------------------------

func TestExternalAuthzService_Check_ResourceMapperError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	accessToken := "test.access.token"
	userSub := "alice@example.com"
	claimsJSON, _ := json.Marshal(map[string]string{"sub": userSub})
	idToken := newTestIDToken(t, claimsJSON)

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), &stsv1.ExchangeRequest{SessionCookie: "mysession"}).
		Return(&stsv1.ExchangeResponse{AccessToken: accessToken}, nil).
		Times(1)
	mockVerifier.EXPECT().
		Verify(gomock.Any(), accessToken).
		Return(idToken, nil).
		Times(1)
	mockResourceMapper.EXPECT().
		Map(gomock.Any(), userSub, "GET", "/api/resource").
		Return(nil, nil, errors.New("mapping failed")).
		Times(1)
	mockOpenFGA.EXPECT().BatchCheck(gomock.Any()).Times(0)

	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

	resp, err := svc.Check(context.Background(), buildCheckRequest(
		map[string]string{"cookie": "session_id=mysession"}, "GET", "/api/resource",
	))
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	deniedResp := resp.GetDeniedResponse()
	if deniedResp == nil {
		t.Fatal("expected DeniedResponse, got nil")
	}
	if deniedResp.Status.Code != envoyType.StatusCode_Forbidden {
		t.Errorf("expected HTTP status %d, got %d", envoyType.StatusCode_Forbidden, deniedResp.Status.Code)
	}
	if deniedResp.Body != "internal error during authorization" {
		t.Errorf("expected body %q, got %q", "internal error during authorization", deniedResp.Body)
	}
	assertRecordCheck(t, metrics, "error", "internal_error")
	assertObserveCalls(t, metrics, 1, 1, 0)
}

func TestExternalAuthzService_Check_NoMatchingRules(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	accessToken := "test.access.token"
	userSub := "alice@example.com"
	claimsJSON, _ := json.Marshal(map[string]string{"sub": userSub})
	idToken := newTestIDToken(t, claimsJSON)

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), &stsv1.ExchangeRequest{SessionCookie: "mysession"}).
		Return(&stsv1.ExchangeResponse{AccessToken: accessToken}, nil).
		Times(1)
	mockVerifier.EXPECT().
		Verify(gomock.Any(), accessToken).
		Return(idToken, nil).
		Times(1)
	// Mapper returns empty slice — no matching rule.
	mockResourceMapper.EXPECT().
		Map(gomock.Any(), userSub, "DELETE", "/api/resource").
		Return([]client.ClientBatchCheckItem{}, nil, nil).
		Times(1)
	mockOpenFGA.EXPECT().BatchCheck(gomock.Any()).Times(0)

	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

	resp, err := svc.Check(context.Background(), buildCheckRequest(
		map[string]string{"cookie": "session_id=mysession"}, "DELETE", "/api/resource",
	))
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	deniedResp := resp.GetDeniedResponse()
	if deniedResp == nil {
		t.Fatal("expected DeniedResponse, got nil")
	}
	if deniedResp.Status.Code != envoyType.StatusCode_Forbidden {
		t.Errorf("expected HTTP status %d, got %d", envoyType.StatusCode_Forbidden, deniedResp.Status.Code)
	}
	expectedBody := fmt.Sprintf("No authorization rule matched method %s path %s", "DELETE", "/api/resource")
	if deniedResp.Body != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, deniedResp.Body)
	}
	assertRecordCheck(t, metrics, "deny", "no_rule_matched")
	assertObserveCalls(t, metrics, 1, 1, 0)
}

// TestExternalAuthzService_Check_QueryStringStripped verifies that a query string
// in the path is stripped before the resource mapper is consulted.
func TestExternalAuthzService_Check_QueryStringStripped(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	accessToken := "test.access.token"
	userSub := "alice@example.com"
	claimsJSON, _ := json.Marshal(map[string]string{"sub": userSub})
	idToken := newTestIDToken(t, claimsJSON)

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), gomock.Any()).
		Return(&stsv1.ExchangeResponse{AccessToken: accessToken}, nil).
		Times(1)
	mockVerifier.EXPECT().
		Verify(gomock.Any(), accessToken).
		Return(idToken, nil).
		Times(1)
	// Mapper must receive path WITHOUT the query string.
	mockResourceMapper.EXPECT().
		Map(gomock.Any(), userSub, "GET", "/api/resource").
		Return([]client.ClientBatchCheckItem{}, nil, nil).
		Times(1)

	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

	// Path contains a query string that should be stripped.
	resp, err := svc.Check(context.Background(), buildCheckRequest(
		map[string]string{"cookie": "session_id=s"}, "GET", "/api/resource?foo=bar&baz=qux",
	))
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	// Result is forbidden because no rules matched — but the important assertion is
	// that Map was called with the stripped path (enforced by the gomock expectation above).
	if resp.GetDeniedResponse() == nil {
		t.Error("expected a denied response")
	}
	assertRecordCheck(t, metrics, "deny", "no_rule_matched")
	assertObserveCalls(t, metrics, 1, 1, 0)
}

// --- Check: OpenFGA batch-check paths ----------------------------------------

func TestExternalAuthzService_Check_FGABatchCheckError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	accessToken := "test.access.token"
	userSub := "alice@example.com"
	claimsJSON, _ := json.Marshal(map[string]string{"sub": userSub})
	idToken := newTestIDToken(t, claimsJSON)

	tuples := []client.ClientBatchCheckItem{
		{User: "user:alice", Relation: "reader", Object: "document:1"},
	}

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), gomock.Any()).
		Return(&stsv1.ExchangeResponse{AccessToken: accessToken}, nil).
		Times(1)
	mockVerifier.EXPECT().
		Verify(gomock.Any(), accessToken).
		Return(idToken, nil).
		Times(1)
	mockResourceMapper.EXPECT().
		Map(gomock.Any(), userSub, "GET", "/api/resource").
		Return(tuples, &rules.RuleWithTuples{Id: "rule-1"}, nil).
		Times(1)

	batchReqMock := &mockBatchCheckRequest{
		response: nil,
		err:      errors.New("fga unavailable"),
	}
	mockOpenFGA.EXPECT().
		BatchCheck(gomock.Any()).
		Return(batchReqMock).
		Times(1)

	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

	resp, err := svc.Check(context.Background(), buildCheckRequest(
		map[string]string{"cookie": "session_id=s"}, "GET", "/api/resource",
	))
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	deniedResp := resp.GetDeniedResponse()
	if deniedResp == nil {
		t.Fatal("expected DeniedResponse, got nil")
	}
	if deniedResp.Status.Code != envoyType.StatusCode_Forbidden {
		t.Errorf("expected HTTP status %d, got %d", envoyType.StatusCode_Forbidden, deniedResp.Status.Code)
	}
	if deniedResp.Body != "authorization check failed" {
		t.Errorf("expected body %q, got %q", "authorization check failed", deniedResp.Body)
	}
	assertRecordCheck(t, metrics, "error", "openfga_error")
	assertObserveCalls(t, metrics, 1, 1, 1)
}

func TestExternalAuthzService_Check_FGAResultNotOK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	accessToken := "test.access.token"
	userSub := "alice@example.com"
	claimsJSON, _ := json.Marshal(map[string]string{"sub": userSub})
	idToken := newTestIDToken(t, claimsJSON)

	tuples := []client.ClientBatchCheckItem{
		{User: "user:alice", Relation: "reader", Object: "document:1"},
	}

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), gomock.Any()).
		Return(&stsv1.ExchangeResponse{AccessToken: accessToken}, nil).
		Times(1)
	mockVerifier.EXPECT().
		Verify(gomock.Any(), accessToken).
		Return(idToken, nil).
		Times(1)
	mockResourceMapper.EXPECT().
		Map(gomock.Any(), userSub, "GET", "/api/resource").
		Return(tuples, &rules.RuleWithTuples{Id: "rule-1"}, nil).
		Times(1)

	// Response with nil Result — GetResultOk() returns false.
	batchReqMock := &mockBatchCheckRequest{
		response: &openfga.BatchCheckResponse{Result: nil},
		err:      nil,
	}
	mockOpenFGA.EXPECT().
		BatchCheck(gomock.Any()).
		Return(batchReqMock).
		Times(1)

	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

	resp, err := svc.Check(context.Background(), buildCheckRequest(
		map[string]string{"cookie": "session_id=s"}, "GET", "/api/resource",
	))
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	deniedResp := resp.GetDeniedResponse()
	if deniedResp == nil {
		t.Fatal("expected DeniedResponse, got nil")
	}
	if deniedResp.Status.Code != envoyType.StatusCode_Forbidden {
		t.Errorf("expected HTTP status %d, got %d", envoyType.StatusCode_Forbidden, deniedResp.Status.Code)
	}
	if deniedResp.Body != "access denied" {
		t.Errorf("expected body %q, got %q", "access denied", deniedResp.Body)
	}
	assertRecordCheck(t, metrics, "error", "openfga_error")
	assertObserveCalls(t, metrics, 1, 1, 1)
}

func TestExternalAuthzService_Check_FGAResultHasError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	accessToken := "test.access.token"
	userSub := "alice@example.com"
	claimsJSON, _ := json.Marshal(map[string]string{"sub": userSub})
	idToken := newTestIDToken(t, claimsJSON)

	tuples := []client.ClientBatchCheckItem{
		{User: "user:alice", Relation: "reader", Object: "document:1"},
	}

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), gomock.Any()).
		Return(&stsv1.ExchangeResponse{AccessToken: accessToken}, nil).
		Times(1)
	mockVerifier.EXPECT().
		Verify(gomock.Any(), accessToken).
		Return(idToken, nil).
		Times(1)
	mockResourceMapper.EXPECT().
		Map(gomock.Any(), userSub, "GET", "/api/resource").
		Return(tuples, &rules.RuleWithTuples{Id: "rule-1"}, nil).
		Times(1)

	checkErr := openfga.CheckError{Message: openfga.PtrString("model evaluation error")}
	resultMap := map[string]openfga.BatchCheckSingleResult{
		"correlation-1": {Error: &checkErr},
	}
	batchReqMock := &mockBatchCheckRequest{
		response: &openfga.BatchCheckResponse{Result: &resultMap},
		err:      nil,
	}
	mockOpenFGA.EXPECT().
		BatchCheck(gomock.Any()).
		Return(batchReqMock).
		Times(1)

	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

	resp, err := svc.Check(context.Background(), buildCheckRequest(
		map[string]string{"cookie": "session_id=s"}, "GET", "/api/resource",
	))
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	deniedResp := resp.GetDeniedResponse()
	if deniedResp == nil {
		t.Fatal("expected DeniedResponse, got nil")
	}
	if deniedResp.Status.Code != envoyType.StatusCode_Forbidden {
		t.Errorf("expected HTTP status %d, got %d", envoyType.StatusCode_Forbidden, deniedResp.Status.Code)
	}
	expectedBody := fmt.Sprintf("access denied for user %s", userSub)
	if deniedResp.Body != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, deniedResp.Body)
	}
	assertRecordCheck(t, metrics, "error", "openfga_error")
	assertObserveCalls(t, metrics, 1, 1, 1)
}

func TestExternalAuthzService_Check_FGAAccessDenied(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	accessToken := "test.access.token"
	userSub := "alice@example.com"
	claimsJSON, _ := json.Marshal(map[string]string{"sub": userSub})
	idToken := newTestIDToken(t, claimsJSON)

	tuples := []client.ClientBatchCheckItem{
		{User: "user:alice", Relation: "reader", Object: "document:1"},
	}

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), gomock.Any()).
		Return(&stsv1.ExchangeResponse{AccessToken: accessToken}, nil).
		Times(1)
	mockVerifier.EXPECT().
		Verify(gomock.Any(), accessToken).
		Return(idToken, nil).
		Times(1)
	mockResourceMapper.EXPECT().
		Map(gomock.Any(), userSub, "GET", "/api/resource").
		Return(tuples, &rules.RuleWithTuples{Id: "rule-1"}, nil).
		Times(1)

	resultMap := map[string]openfga.BatchCheckSingleResult{
		"correlation-1": {Allowed: boolPtr(false)},
	}
	batchReqMock := &mockBatchCheckRequest{
		response: &openfga.BatchCheckResponse{Result: &resultMap},
		err:      nil,
	}
	mockOpenFGA.EXPECT().
		BatchCheck(gomock.Any()).
		Return(batchReqMock).
		Times(1)

	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

	resp, err := svc.Check(context.Background(), buildCheckRequest(
		map[string]string{"cookie": "session_id=s"}, "GET", "/api/resource",
	))
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	deniedResp := resp.GetDeniedResponse()
	if deniedResp == nil {
		t.Fatal("expected DeniedResponse, got nil")
	}
	if deniedResp.Status.Code != envoyType.StatusCode_Forbidden {
		t.Errorf("expected HTTP status %d, got %d", envoyType.StatusCode_Forbidden, deniedResp.Status.Code)
	}
	expectedBody := fmt.Sprintf("access denied for user %s", userSub)
	if deniedResp.Body != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, deniedResp.Body)
	}
	assertRecordCheck(t, metrics, "deny", "openfga_denied")
	assertObserveCalls(t, metrics, 1, 1, 1)
}

// --- Check: full success path ------------------------------------------------

// TestExternalAuthzService_Check_FullSuccess exercises the entire happy path:
// valid session cookie → STS exchange → JWT verification → resource mapping →
// OpenFGA batch check returns allowed → 200 with Bearer token.
func TestExternalAuthzService_Check_FullSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	accessToken := "test.access.token"
	userSub := "alice@example.com"
	claimsJSON, _ := json.Marshal(map[string]string{"sub": userSub})
	idToken := newTestIDToken(t, claimsJSON)

	tuples := []client.ClientBatchCheckItem{
		{User: "user:alice", Relation: "reader", Object: "document:1"},
	}

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), &stsv1.ExchangeRequest{SessionCookie: "valid-session"}).
		Return(&stsv1.ExchangeResponse{AccessToken: accessToken}, nil).
		Times(1)
	mockVerifier.EXPECT().
		Verify(gomock.Any(), accessToken).
		Return(idToken, nil).
		Times(1)
	mockResourceMapper.EXPECT().
		Map(gomock.Any(), userSub, "GET", "/api/resource").
		Return(tuples, &rules.RuleWithTuples{Id: "rule-1"}, nil).
		Times(1)

	resultMap := map[string]openfga.BatchCheckSingleResult{
		"correlation-1": {Allowed: boolPtr(true)},
	}
	batchReqMock := &mockBatchCheckRequest{
		response: &openfga.BatchCheckResponse{Result: &resultMap},
		err:      nil,
	}
	mockOpenFGA.EXPECT().
		BatchCheck(gomock.Any()).
		Return(batchReqMock).
		Times(1)

	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

	resp, err := svc.Check(context.Background(), buildCheckRequest(
		map[string]string{"cookie": "session_id=valid-session"}, "GET", "/api/resource",
	))
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
		t.Errorf("expected header key 'authorization', got %q", authHeader.Key)
	}
	if authHeader.Value != "Bearer "+accessToken {
		t.Errorf("expected header value %q, got %q", "Bearer "+accessToken, authHeader.Value)
	}
	assertRecordCheck(t, metrics, "allow", "ok")
	assertObserveCalls(t, metrics, 1, 1, 1)
}

// --- Check: "Authorization decision" log line --------------------------------

// TestExternalAuthzService_Check_LogsAuthorizationDecision_Allow mirrors
// TestExternalAuthzService_Check_FullSuccess but asserts on the captured log
// output rather than the response.
func TestExternalAuthzService_Check_LogsAuthorizationDecision_Allow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	accessToken := "test.access.token"
	userSub := "alice@example.com"
	claimsJSON, _ := json.Marshal(map[string]string{"sub": userSub})
	idToken := newTestIDToken(t, claimsJSON)

	tuples := []client.ClientBatchCheckItem{
		{User: "user:alice", Relation: "reader", Object: "document:1"},
	}

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), &stsv1.ExchangeRequest{SessionCookie: "valid-session"}).
		Return(&stsv1.ExchangeResponse{AccessToken: accessToken}, nil).
		Times(1)
	mockVerifier.EXPECT().
		Verify(gomock.Any(), accessToken).
		Return(idToken, nil).
		Times(1)
	mockResourceMapper.EXPECT().
		Map(gomock.Any(), userSub, "GET", "/api/resource").
		Return(tuples, &rules.RuleWithTuples{Id: "rule-1"}, nil).
		Times(1)

	resultMap := map[string]openfga.BatchCheckSingleResult{
		"correlation-1": {Allowed: boolPtr(true)},
	}
	batchReqMock := &mockBatchCheckRequest{
		response: &openfga.BatchCheckResponse{Result: &resultMap},
		err:      nil,
	}
	mockOpenFGA.EXPECT().
		BatchCheck(gomock.Any()).
		Return(batchReqMock).
		Times(1)

	logger, buf := testutil.CapturingLogger(t)
	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, logger, noop.NewTracerProvider().Tracer("test"))

	ctx := logging.ContextWithRequestID(context.Background(), "req-allow-1")
	_, err := svc.Check(ctx, buildCheckRequest(
		map[string]string{"cookie": "session_id=valid-session"}, "GET", "/api/resource?foo=bar",
	))
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	entry := decodeAuthzDecisionLine(t, buf)
	wantFields := map[string]any{
		"request_id": "req-allow-1",
		"result":     "allow",
		"reason":     "ok",
		"method":     "GET",
		"path":       "/api/resource",
		"subject":    userSub,
		"tenant":     "",
	}
	for field, want := range wantFields {
		if got := entry[field]; got != want {
			t.Errorf("field %q: expected %v, got %v", field, want, got)
		}
	}
	if _, ok := entry["duration_ms"]; !ok {
		t.Error("expected \"duration_ms\" field in log entry")
	}
	if _, ok := entry["error"]; ok {
		t.Errorf("expected no \"error\" field on the success path, got %v", entry["error"])
	}
}

// TestExternalAuthzService_Check_LogsAuthorizationDecision_Deny mirrors
// TestExternalAuthzService_Check_FGAAccessDenied but asserts on the captured
// log output rather than the response.
func TestExternalAuthzService_Check_LogsAuthorizationDecision_Deny(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	accessToken := "test.access.token"
	userSub := "alice@example.com"
	claimsJSON, _ := json.Marshal(map[string]string{"sub": userSub})
	idToken := newTestIDToken(t, claimsJSON)

	tuples := []client.ClientBatchCheckItem{
		{User: "user:alice", Relation: "reader", Object: "document:1"},
	}

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), gomock.Any()).
		Return(&stsv1.ExchangeResponse{AccessToken: accessToken}, nil).
		Times(1)
	mockVerifier.EXPECT().
		Verify(gomock.Any(), accessToken).
		Return(idToken, nil).
		Times(1)
	mockResourceMapper.EXPECT().
		Map(gomock.Any(), userSub, "GET", "/api/resource").
		Return(tuples, &rules.RuleWithTuples{Id: "rule-1"}, nil).
		Times(1)

	resultMap := map[string]openfga.BatchCheckSingleResult{
		"correlation-1": {Allowed: boolPtr(false)},
	}
	batchReqMock := &mockBatchCheckRequest{
		response: &openfga.BatchCheckResponse{Result: &resultMap},
		err:      nil,
	}
	mockOpenFGA.EXPECT().
		BatchCheck(gomock.Any()).
		Return(batchReqMock).
		Times(1)

	logger, buf := testutil.CapturingLogger(t)
	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, logger, noop.NewTracerProvider().Tracer("test"))

	ctx := logging.ContextWithRequestID(context.Background(), "req-deny-1")
	_, err := svc.Check(ctx, buildCheckRequest(
		map[string]string{"cookie": "session_id=s"}, "GET", "/api/resource",
	))
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	entry := decodeAuthzDecisionLine(t, buf)
	wantFields := map[string]any{
		"request_id": "req-deny-1",
		"result":     "deny",
		"reason":     "openfga_denied",
		"method":     "GET",
		"path":       "/api/resource",
		"subject":    userSub,
	}
	for field, want := range wantFields {
		if got := entry[field]; got != want {
			t.Errorf("field %q: expected %v, got %v", field, want, got)
		}
	}
	if _, ok := entry["error"]; ok {
		t.Errorf("expected no \"error\" field on the deny path, got %v", entry["error"])
	}
}

// TestExternalAuthzService_Check_LogsAuthorizationDecision_Error mirrors
// TestExternalAuthzService_Check_MissingMatchedRule but asserts on the
// captured log output rather than the response. This is the one internal_error
// branch where check() returns a non-nil error (a nil matchedRule despite
// non-empty tuples is a resourceMapper/rule-data bug, not just a denial),
// which is what makes the "error" field on the log line non-empty.
func TestExternalAuthzService_Check_LogsAuthorizationDecision_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	accessToken := "test.access.token"
	userSub := "alice@example.com"
	claimsJSON, _ := json.Marshal(map[string]string{"sub": userSub})
	idToken := newTestIDToken(t, claimsJSON)

	tuples := []client.ClientBatchCheckItem{
		{User: "user:alice", Relation: "reader", Object: "document:1"},
	}

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), gomock.Any()).
		Return(&stsv1.ExchangeResponse{AccessToken: accessToken}, nil).
		Times(1)
	mockVerifier.EXPECT().
		Verify(gomock.Any(), accessToken).
		Return(idToken, nil).
		Times(1)
	// Return non-empty tuples but nil matchedRule.
	mockResourceMapper.EXPECT().
		Map(gomock.Any(), userSub, "GET", "/api/resource").
		Return(tuples, nil, nil).
		Times(1)

	logger, buf := testutil.CapturingLogger(t)
	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, logger, noop.NewTracerProvider().Tracer("test"))

	ctx := logging.ContextWithRequestID(context.Background(), "req-error-1")
	_, err := svc.Check(ctx, buildCheckRequest(
		map[string]string{"cookie": "session_id=valid-session"}, "GET", "/api/resource",
	))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	entry := decodeAuthzDecisionLine(t, buf)
	wantFields := map[string]any{
		"request_id": "req-error-1",
		"result":     "error",
		"reason":     "internal_error",
		"method":     "GET",
		"path":       "/api/resource",
		"subject":    userSub,
	}
	for field, want := range wantFields {
		if got := entry[field]; got != want {
			t.Errorf("field %q: expected %v, got %v", field, want, got)
		}
	}
	if entry["error"] == nil || entry["error"] == "" {
		t.Errorf("expected non-empty \"error\" field on the error path, got %v", entry["error"])
	}
}

// TestExternalAuthzService_Check_NoDuplicateDebugLogOnExchangeSessionError
// guards against the previously-removed
// "Failed to exchange session cookie" Debug duplicate reappearing on the
// sts_exchange_failed path — internal/integration/sts/client.go already logs
// the same error at Error level with more detail.
func TestExternalAuthzService_Check_NoDuplicateDebugLogOnExchangeSessionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), &stsv1.ExchangeRequest{SessionCookie: "invalid-session"}).
		Return(nil, errors.New("invalid session")).
		Times(1)
	mockVerifier.EXPECT().Verify(gomock.Any(), gomock.Any()).Times(0)

	logger, buf := debugCapturingLogger(t)
	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, logger, noop.NewTracerProvider().Tracer("test"))

	_, err := svc.Check(context.Background(), buildCheckRequest(
		map[string]string{"cookie": "session_id=invalid-session"}, "GET", "/api",
	))
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	for _, entry := range decodeLogLines(t, buf) {
		if entry["msg"] == "Failed to exchange session cookie" {
			t.Fatalf("unexpected duplicate log line %q found: %v", entry["msg"], entry)
		}
	}
	decodeAuthzDecisionLine(t, buf)
}

// --- Check: cookie-parsing edge cases ----------------------------------------

func TestExternalAuthzService_Check_MultipleCookies(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	expectedToken := "test.jwt.token"

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), &stsv1.ExchangeRequest{SessionCookie: "mysessionvalue"}).
		Return(&stsv1.ExchangeResponse{AccessToken: expectedToken, ExpiresIn: 1800}, nil).
		Times(1)
	// Token verification fails — returns unauthorized.
	mockVerifier.EXPECT().
		Verify(gomock.Any(), expectedToken).
		Return(nil, fmt.Errorf("token does not contain a 'sub' claim")).
		Times(1)

	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

	resp, err := svc.Check(context.Background(), buildCheckRequest(
		map[string]string{"cookie": "foo=bar; session_id=mysessionvalue; baz=qux"}, "GET", "/api",
	))
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
	assertRecordCheck(t, metrics, "deny", "jwt_invalid")
	assertObserveCalls(t, metrics, 1, 0, 0)
}

func TestExternalAuthzService_Check_CookieWithSpaces(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	expectedToken := "jwt.with.spaces"

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), &stsv1.ExchangeRequest{SessionCookie: "spaced-value"}).
		Return(&stsv1.ExchangeResponse{AccessToken: expectedToken, ExpiresIn: 7200}, nil).
		Times(1)
	mockVerifier.EXPECT().
		Verify(gomock.Any(), expectedToken).
		Return(nil, fmt.Errorf("token does not contain a 'sub' claim")).
		Times(1)

	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

	resp, err := svc.Check(context.Background(), buildCheckRequest(
		map[string]string{"cookie": "  foo=bar  ;  session_id=spaced-value  ;  baz=qux  "}, "GET", "/api",
	))
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if resp.Status.Code != int32(codes.Unauthenticated) {
		t.Errorf("expected status code %d, got %d", codes.Unauthenticated, resp.Status.Code)
	}
	assertRecordCheck(t, metrics, "deny", "jwt_invalid")
	assertObserveCalls(t, metrics, 1, 0, 0)
}

// --- Register ----------------------------------------------------------------

func TestExternalAuthzService_Register(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	mockVerifier.EXPECT().Verify(gomock.Any(), gomock.Any()).Times(0)

	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, nil, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

	// Verify the service implements the interface — compile-time check suffices.
	var _ ExternalAuthzServiceInterface = svc
}

// --- Unit tests for helper functions -----------------------------------------

func TestExtractSessionCookie(t *testing.T) {
	tests := []struct {
		name          string
		cookieHeader  string
		expectedValue string
	}{
		{
			name:          "single session cookie",
			cookieHeader:  "session_id=abc123",
			expectedValue: "abc123",
		},
		{
			name:          "session cookie with multiple cookies",
			cookieHeader:  "foo=bar; session_id=xyz789; baz=qux",
			expectedValue: "xyz789",
		},
		{
			name:          "session cookie at start",
			cookieHeader:  "session_id=first; other=value",
			expectedValue: "first",
		},
		{
			name:          "session cookie at end",
			cookieHeader:  "other=value; session_id=last",
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
			cookieHeader:  "  session_id=spaced  ",
			expectedValue: "spaced",
		},
		{
			name:          "session cookie with complex value",
			cookieHeader:  "session_id=value-with-dashes_and_underscores.and.dots",
			expectedValue: "value-with-dashes_and_underscores.and.dots",
		},
		{
			name:          "similar cookie name",
			cookieHeader:  "session-id=wrong; session_id=correct",
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
			cookieHeader:  "session_id=abc123",
			expectedCount: 1,
			expectedFirst: "session_id=abc123",
		},
		{
			name:          "multiple cookies",
			cookieHeader:  "foo=bar; baz=qux; session_id=xyz",
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
		{name: "standard unauthorized message", body: "Unauthorized access"},
		{name: "no session cookie provided", body: "No session cookie provided"},
		{name: "empty message", body: ""},
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
		{name: "standard forbidden message", body: "Access forbidden"},
		{name: "invalid session error", body: "invalid session"},
		{name: "empty message", body: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := forbidden(tt.body)
			if resp == nil {
				t.Fatal("forbidden returned nil")
			}
			if resp.Status.Code != int32(codes.PermissionDenied) {
				t.Errorf("expected status code %d, got %d", codes.PermissionDenied, resp.Status.Code)
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
		name             string
		body             string
		expectedStatus   envoyType.StatusCode
		expectedGRPCCode codes.Code
	}{
		{name: "unauthorized error", body: "Unauthorized", expectedStatus: envoyType.StatusCode_Unauthorized, expectedGRPCCode: codes.Unauthenticated},
		{name: "forbidden error", body: "Forbidden", expectedStatus: envoyType.StatusCode_Forbidden, expectedGRPCCode: codes.PermissionDenied},
		{name: "custom error message", body: "Invalid session", expectedStatus: envoyType.StatusCode_Forbidden, expectedGRPCCode: codes.PermissionDenied},
		{name: "empty message", body: "", expectedStatus: envoyType.StatusCode_Unauthorized, expectedGRPCCode: codes.Unauthenticated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := denyResponse(tt.body, tt.expectedStatus)
			if resp == nil {
				t.Fatal("denyResponse returned nil")
			}
			if resp.Status.Code != int32(tt.expectedGRPCCode) {
				t.Errorf("expected status code %d, got %d", tt.expectedGRPCCode, resp.Status.Code)
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
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)
	logger := testLoggerExternal(t)
	tracer := noop.NewTracerProvider().Tracer("test")

	mockVerifier.EXPECT().Verify(gomock.Any(), gomock.Any()).Times(0)

	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, nil, logger, tracer)

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

func TestExternalAuthzService_Check_Multitenancy(t *testing.T) {
	tests := []struct {
		name                string
		multitenancyEnabled bool
		expectedEnabled     bool
	}{
		{
			name:                "Multitenancy enabled",
			multitenancyEnabled: true,
			expectedEnabled:     true,
		},
		{
			name:                "Multitenancy disabled",
			multitenancyEnabled: false,
			expectedEnabled:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
			mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
			mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
			mockVerifier := authz.NewMockoidcVerifier(ctrl)

			accessToken := "test.access.token"
			userSub := "alice@example.com"
			orgID := "Canonical"
			claimsJSON, _ := json.Marshal(map[string]string{"sub": userSub, "org": orgID})
			idToken := newTestIDToken(t, claimsJSON)

			tuples := []client.ClientBatchCheckItem{
				{User: "user:alice", Relation: "reader", Object: "document:1"},
			}

			mockSTS.EXPECT().
				ExchangeSession(gomock.Any(), &stsv1.ExchangeRequest{SessionCookie: "valid-session"}).
				Return(&stsv1.ExchangeResponse{AccessToken: accessToken}, nil).
				Times(1)
			mockVerifier.EXPECT().
				Verify(gomock.Any(), accessToken).
				Return(idToken, nil).
				Times(1)
			tenantVal := orgID
			mockResourceMapper.EXPECT().
				Map(gomock.Any(), userSub, "GET", "/api/resource").
				Return(tuples, &rules.RuleWithTuples{Id: "rule-1", Tenant: &tenantVal}, nil).
				Times(1)

			resultMap := map[string]openfga.BatchCheckSingleResult{
				"correlation-1": {Allowed: boolPtr(true)},
			}
			batchReqMock := &mockBatchCheckRequest{
				response: &openfga.BatchCheckResponse{Result: &resultMap},
				err:      nil,
			}

			// Capture and verify the contextual params in BatchCheck
			mockOpenFGA.EXPECT().
				BatchCheck(gomock.Any()).
				Do(func(ctx context.Context) {
					// We can check that the context has been set on tuples
					if len(tuples) != 1 {
						t.Fatalf("expected 1 tuple, got %d", len(tuples))
					}
					if tuples[0].Context == nil {
						t.Fatal("expected Context on tuple but got nil")
					}
					ctxMap := *tuples[0].Context
					if ctxMap["tenant_enabled"] != tc.expectedEnabled {
						t.Errorf("expected tenant_enabled to be %t, got %v", tc.expectedEnabled, ctxMap["tenant_enabled"])
					}
					if ctxMap["user_tenant"] != orgID {
						t.Errorf("expected user_tenant to be %q, got %v", orgID, ctxMap["user_tenant"])
					}
				}).
				Return(batchReqMock).
				Times(1)

			metrics := &fakeCheckMetrics{}
			svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, tc.multitenancyEnabled, metrics, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

			resp, err := svc.Check(context.Background(), buildCheckRequest(
				map[string]string{"cookie": "session_id=valid-session"}, "GET", "/api/resource",
			))
			if err != nil {
				t.Fatalf("Check failed: %v", err)
			}
			if resp.Status.Code != int32(codes.OK) {
				t.Errorf("expected status code %d, got %d", codes.OK, resp.Status.Code)
			}
			assertRecordCheck(t, metrics, "allow", "ok")
			assertObserveCalls(t, metrics, 1, 1, 1)
		})
	}
}

func TestExternalAuthzService_Check_Multitenancy_MissingTenant(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	accessToken := "test.access.token"
	userSub := "alice@example.com"
	claimsJSON, _ := json.Marshal(map[string]string{"sub": userSub, "org": "Canonical"})
	idToken := newTestIDToken(t, claimsJSON)

	tuples := []client.ClientBatchCheckItem{
		{User: "user:alice", Relation: "reader", Object: "document:1"},
	}

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), gomock.Any()).
		Return(&stsv1.ExchangeResponse{AccessToken: accessToken}, nil).
		Times(1)
	mockVerifier.EXPECT().
		Verify(gomock.Any(), accessToken).
		Return(idToken, nil).
		Times(1)
	// Return matched rule with nil/empty tenant
	mockResourceMapper.EXPECT().
		Map(gomock.Any(), userSub, "GET", "/api/resource").
		Return(tuples, &rules.RuleWithTuples{Id: "rule-1", Tenant: nil}, nil).
		Times(1)

	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, true, metrics, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

	resp, err := svc.Check(context.Background(), buildCheckRequest(
		map[string]string{"cookie": "session_id=valid-session"}, "GET", "/api/resource",
	))

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}
	assertRecordCheck(t, metrics, "error", "internal_error")
	assertObserveCalls(t, metrics, 1, 1, 0)
}

func TestExternalAuthzService_Check_MissingMatchedRule(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSTS := authz.NewMockSecurityTokenServiceClient(ctrl)
	mockResourceMapper := authz.NewMockResourceMapperInterface(ctrl)
	mockOpenFGA := authz.NewMockOpenFGAClientInterface(ctrl)
	mockVerifier := authz.NewMockoidcVerifier(ctrl)

	accessToken := "test.access.token"
	userSub := "alice@example.com"
	claimsJSON, _ := json.Marshal(map[string]string{"sub": userSub, "org": "Canonical"})
	idToken := newTestIDToken(t, claimsJSON)

	tuples := []client.ClientBatchCheckItem{
		{User: "user:alice", Relation: "reader", Object: "document:1"},
	}

	mockSTS.EXPECT().
		ExchangeSession(gomock.Any(), gomock.Any()).
		Return(&stsv1.ExchangeResponse{AccessToken: accessToken}, nil).
		Times(1)
	mockVerifier.EXPECT().
		Verify(gomock.Any(), accessToken).
		Return(idToken, nil).
		Times(1)
	// Return non-empty tuples but nil matchedRule
	mockResourceMapper.EXPECT().
		Map(gomock.Any(), userSub, "GET", "/api/resource").
		Return(tuples, nil, nil).
		Times(1)

	metrics := &fakeCheckMetrics{}
	svc := NewExternalAuthzService(mockVerifier, mockSTS, mockResourceMapper, mockOpenFGA, false, metrics, testLoggerExternal(t), noop.NewTracerProvider().Tracer("test"))

	resp, err := svc.Check(context.Background(), buildCheckRequest(
		map[string]string{"cookie": "session_id=valid-session"}, "GET", "/api/resource",
	))

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no matching rule found") {
		t.Errorf("expected error message to contain 'no matching rule found', got %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}
	assertRecordCheck(t, metrics, "error", "internal_error")
	assertObserveCalls(t, metrics, 1, 1, 0)
}
