// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

//go:generate mockgen -source=../integration/postgres/client.go -destination=mocks/mock_postgres.go -package=repository

package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/pashagolub/pgxmock/v5"
	"go.uber.org/mock/gomock"

	repository "github.com/canonical/authorization-service/internal/repository/mocks"
)

// helper creates a gomock-based MockDBClientInterface with a real pgxmock pool
// for row creation. The Builder() is wired to return a Dollar-placeholder builder.
func setupMocks(t *testing.T) (*repository.MockDBClientInterface, pgxmock.PgxPoolIface) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockDB := repository.NewMockDBClientInterface(ctrl)
	pool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create pgxmock pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Default: Builder() returns a Postgres-style dollar-placeholder builder.
	mockDB.EXPECT().Builder().Return(sq.StatementBuilder.PlaceholderFormat(sq.Dollar)).AnyTimes()

	return mockDB, pool
}

// ── countSegments ─────────────────────────────────────────────────────────────

func TestCountSegments(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected int
	}{
		{"typical path", "/api/v1/groups/123", 4},
		{"root", "/", 0},
		{"empty string", "", 0},
		{"trailing slash", "/api/v1/", 2},
		{"double slashes", "//api///v1//", 2},
		{"no leading slash", "api/v1", 2},
		{"single segment", "/users", 1},
		{"deeply nested", "/a/b/c/d/e/f", 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countSegments(tt.path)
			if got != tt.expected {
				t.Errorf("countSegments(%q) = %d, want %d", tt.path, got, tt.expected)
			}
		})
	}
}

// ── buildPrefixes ────────────────────────────────────────────────────────────

func TestBuildPrefixes(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []string
	}{
		{
			name:     "root path returns single slash",
			path:     "/",
			expected: []string{"/"},
		},
		{
			name:     "typical multi-segment path",
			path:     "/api/v1/groups/group-id-1/members",
			expected: []string{"/api/", "/api/v1/", "/api/v1/groups/", "/api/v1/groups/group-id-1/", "/api/v1/groups/group-id-1/members"},
		},
		{
			name:     "trailing slash preserved on last segment",
			path:     "/api/v1/",
			expected: []string{"/api/", "/api/v1/"},
		},
		{
			name:     "single segment with leading slash",
			path:     "/users",
			expected: []string{"/users"},
		},
		{
			name:     "single segment without leading slash",
			path:     "users",
			expected: []string{"users"},
		},
		{
			name:     "two segments without leading slash",
			path:     "api/v1",
			expected: []string{"api/", "api/v1"},
		},
		{
			name:     "single segment with trailing slash",
			path:     "/api/",
			expected: []string{"/api/"},
		},
		{
			name:     "deeply nested path",
			path:     "/a/b/c/d/e/f",
			expected: []string{"/a/", "/a/b/", "/a/b/c/", "/a/b/c/d/", "/a/b/c/d/e/", "/a/b/c/d/e/f"},
		},
		{
			name:     "two segments with leading slash",
			path:     "/api/v1",
			expected: []string{"/api/", "/api/v1"},
		},
		{
			name:     "path without leading slash and with trailing slash",
			path:     "api/v1/",
			expected: []string{"api/", "api/v1/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPrefixes(tt.path, countSegments(tt.path))

			if len(got) != len(tt.expected) {
				t.Fatalf("buildPrefixes(%q) returned %d prefixes, want %d\ngot:  %v\nwant: %v",
					tt.path, len(got), len(tt.expected), got, tt.expected)
			}

			for i := range tt.expected {
				if got[i] != tt.expected[i] {
					t.Errorf("buildPrefixes(%q)[%d] = %q, want %q", tt.path, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// ── buildFindCandidatesQuery ─────────────────────────────────────────────────

func TestBuildFindCandidatesQuery(t *testing.T) {
	mockDB, _ := setupMocks(t)
	repo := NewPostgresRuleRepository(mockDB, nil)

	sql, args, err := repo.buildFindCandidatesQuery("GET", []string{"/api", "/api/v1", "/api/v1/groups", "/api/v1/groups/123"}, 4)
	if err != nil {
		t.Fatalf("buildFindCandidatesQuery returned error: %v", err)
	}

	expectedSQL := "SELECT r.id, r.service_id, r.method, r.segment_count, r.static_prefix, r.path_regex, r.priority, " +
		"s.slug, s.tenant, " +
		"r.revision, " +
		"t.id, t.rule_id, t.user_resource_type, t.permission, t.object_resource_type, t.object_resource_id " +
		"FROM authorization_rule r " +
		"JOIN authorization_rule_tuple t ON t.rule_id = r.id " +
		"JOIN federated_service s ON s.id = r.service_id " +
		"WHERE r.method = $1 AND r.static_prefix = ANY($2) AND r.segment_count <= $3 " +
		"ORDER BY r.segment_count DESC," +
		" r.priority ASC, r.id ASC"

	if sql != expectedSQL {
		t.Errorf("unexpected SQL:\ngot:  %s\nwant: %s", sql, expectedSQL)
	}

	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d: %v", len(args), args)
	}
	if args[0] != "GET" {
		t.Errorf("args[0] = %v, want %q", args[0], "GET")
	}
	prefixes, ok := args[1].([]string)
	if !ok {
		t.Fatalf("args[1] is %T, want []string", args[1])
	}
	expectedPrefixes := []string{"/api", "/api/v1", "/api/v1/groups", "/api/v1/groups/123"}
	if len(prefixes) != len(expectedPrefixes) {
		t.Fatalf("args[1] has %d prefixes, want %d: %v", len(prefixes), len(expectedPrefixes), prefixes)
	}
	for i, p := range expectedPrefixes {
		if prefixes[i] != p {
			t.Errorf("args[1][%d] = %q, want %q", i, prefixes[i], p)
		}
	}
	if args[2] != 4 {
		t.Errorf("args[2] = %v, want %d", args[2], 4)
	}
}

// ── NewPostgresRuleRepository ────────────────────────────────────────────────

func TestNewPostgresRuleRepository(t *testing.T) {
	mockDB, _ := setupMocks(t)
	repo := NewPostgresRuleRepository(mockDB, nil)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}

// ── FindCandidates ───────────────────────────────────────────────────────────

func TestFindCandidates_EmptyInputs(t *testing.T) {
	mockDB, _ := setupMocks(t)
	repo := NewPostgresRuleRepository(mockDB, nil)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"empty method", "", "/api/v1"},
		{"empty path", "GET", ""},
		{"both empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.FindCandidates(context.Background(), tt.method, tt.path)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if result != nil {
				t.Errorf("expected nil result, got %v", result)
			}
			if !strings.Contains(err.Error(), "method and path cannot be empty") {
				t.Errorf("unexpected error message: %v", err)
			}
		})
	}
}

var ruleColumns = []string{
	"id", "service_id", "method", "segment_count", "static_prefix", "path_regex", "priority",
	"slug", "tenant",
	"revision",
	"id", "rule_id", "user_resource_type", "permission", "object_resource_type", "object_resource_id",
}

func TestFindCandidates_SingleRuleSingleTuple(t *testing.T) {
	mockDB, pool := setupMocks(t)
	metrics := &fakeQueryMetrics{}
	repo := NewPostgresRuleRepository(mockDB, metrics)

	rows := pool.NewRows(ruleColumns).AddRow(
		"1", "10", "GET", 4, "/api/v1/groups/", `^/api/v1/groups/(?<groupId>\d+)$`, 0,
		"payments", nil,
		"rev-1",
		"100", "1", "user", "edit", "group", "{groupId}",
	)

	mockDB.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
			pool.ExpectQuery(".*").WillReturnRows(rows)
			return pool.Query(ctx, "test")
		})

	result, err := repo.FindCandidates(context.Background(), "GET", "/api/v1/groups/123")
	if err != nil {
		t.Fatalf("FindCandidates returned error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(result))
	}

	r := result[0]
	if r.Id != "1" {
		t.Errorf("rule Id = %q, want %q", r.Id, "1")
	}
	if r.ServiceId != "10" {
		t.Errorf("rule ServiceId = %q, want %q", r.ServiceId, "10")
	}
	if r.Method != "GET" {
		t.Errorf("rule Method = %q, want %q", r.Method, "GET")
	}
	if r.SegmentCount != 4 {
		t.Errorf("rule SegmentCount = %d, want %d", r.SegmentCount, 4)
	}
	if r.StaticPrefix != "/api/v1/groups/" {
		t.Errorf("rule StaticPrefix = %q, want %q", r.StaticPrefix, "/api/v1/groups/")
	}
	if r.Priority != 0 {
		t.Errorf("rule Priority = %d, want %d", r.Priority, 0)
	}

	if len(r.RuleTuples) != 1 {
		t.Fatalf("expected 1 tuple, got %d", len(r.RuleTuples))
	}
	tup := r.RuleTuples[0]
	if tup.Id != "100" {
		t.Errorf("tuple Id = %q, want %q", tup.Id, "100")
	}
	if tup.RuleId != "1" {
		t.Errorf("tuple RuleId = %q, want %q", tup.RuleId, "1")
	}
	if tup.UserResourceType != "user" {
		t.Errorf("tuple UserResourceType = %q, want %q", tup.UserResourceType, "user")
	}
	if tup.Permission != "edit" {
		t.Errorf("tuple Permission = %q, want %q", tup.Permission, "edit")
	}
	if tup.ObjectResourceType != "group" {
		t.Errorf("tuple ObjectResourceType = %q, want %q", tup.ObjectResourceType, "group")
	}
	if tup.ObjectResourceId != "{groupId}" {
		t.Errorf("tuple ObjectResourceId = %q, want %q", tup.ObjectResourceId, "{groupId}")
	}
	assertObserveQuery(t, metrics, "find_candidates", false)
}

func TestFindCandidates_SingleRuleMultipleTuples(t *testing.T) {
	mockDB, pool := setupMocks(t)
	repo := NewPostgresRuleRepository(mockDB, nil)

	rows := pool.NewRows(ruleColumns).
		AddRow("1", "10", "POST", 3, "/api/v1/", `^/api/v1/items$`, 0, "payments", nil, "rev-1", "100", "1", "user", "read", "item", "static-val").
		AddRow("1", "10", "POST", 3, "/api/v1/", `^/api/v1/items$`, 0, "payments", nil, "rev-1", "101", "1", "admin", "write", "item", "other-val")

	mockDB.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
			pool.ExpectQuery(".*").WillReturnRows(rows)
			return pool.Query(ctx, "test")
		})

	result, err := repo.FindCandidates(context.Background(), "POST", "/api/v1/items")
	if err != nil {
		t.Fatalf("FindCandidates returned error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(result))
	}
	if len(result[0].RuleTuples) != 2 {
		t.Fatalf("expected 2 tuples, got %d", len(result[0].RuleTuples))
	}
	if result[0].RuleTuples[0].Id != "100" {
		t.Errorf("first tuple Id = %q, want %q", result[0].RuleTuples[0].Id, "100")
	}
	if result[0].RuleTuples[1].Id != "101" {
		t.Errorf("second tuple Id = %q, want %q", result[0].RuleTuples[1].Id, "101")
	}
}

func TestFindCandidates_MultipleRules(t *testing.T) {
	mockDB, pool := setupMocks(t)
	repo := NewPostgresRuleRepository(mockDB, nil)

	rows := pool.NewRows(ruleColumns).
		AddRow("1", "10", "GET", 2, "/api/", `^/api/v1$`, 0, "payments", nil, "rev-1", "100", "1", "user", "read", "api", "v1").
		AddRow("2", "10", "GET", 2, "/api/", `^/api/v2$`, 1, "payments", nil, "rev-2", "200", "2", "user", "read", "api", "v2")

	mockDB.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
			pool.ExpectQuery(".*").WillReturnRows(rows)
			return pool.Query(ctx, "test")
		})

	result, err := repo.FindCandidates(context.Background(), "GET", "/api/v1")
	if err != nil {
		t.Fatalf("FindCandidates returned error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(result))
	}
	if result[0].Id != "1" {
		t.Errorf("first rule Id = %q, want %q", result[0].Id, "1")
	}
	if result[1].Id != "2" {
		t.Errorf("second rule Id = %q, want %q", result[1].Id, "2")
	}
}

func TestFindCandidates_NoRows(t *testing.T) {
	mockDB, pool := setupMocks(t)
	repo := NewPostgresRuleRepository(mockDB, nil)

	rows := pool.NewRows(ruleColumns)

	mockDB.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
			pool.ExpectQuery(".*").WillReturnRows(rows)
			return pool.Query(ctx, "test")
		})

	result, err := repo.FindCandidates(context.Background(), "DELETE", "/nothing")
	if err != nil {
		t.Fatalf("FindCandidates returned error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 rules, got %d", len(result))
	}
}

func TestFindCandidates_QueryError(t *testing.T) {
	mockDB, _ := setupMocks(t)
	metrics := &fakeQueryMetrics{}
	repo := NewPostgresRuleRepository(mockDB, metrics)

	dbErr := errors.New("connection refused")
	mockDB.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, dbErr)

	result, err := repo.FindCandidates(context.Background(), "GET", "/api/v1")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on error, got %v", result)
	}
	if !errors.Is(err, dbErr) {
		t.Errorf("expected error to wrap %v, got %v", dbErr, err)
	}
	assertObserveQuery(t, metrics, "find_candidates", true)
}

func TestFindCandidates_ScanError(t *testing.T) {
	mockDB, pool := setupMocks(t)
	repo := NewPostgresRuleRepository(mockDB, nil)

	// Return a row with wrong number of columns to trigger a scan error.
	rows := pool.NewRows([]string{"id"}).AddRow("bad")

	mockDB.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
			pool.ExpectQuery(".*").WillReturnRows(rows)
			return pool.Query(ctx, "test")
		})

	result, err := repo.FindCandidates(context.Background(), "GET", "/x")
	if err == nil {
		t.Fatal("expected a scan error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on scan error, got %v", result)
	}
}

func TestFindCandidates_RowsError(t *testing.T) {
	mockDB, pool := setupMocks(t)
	repo := NewPostgresRuleRepository(mockDB, nil)

	rowErr := errors.New("unexpected EOF")
	rows := pool.NewRows(ruleColumns).
		AddRow("1", "10", "GET", 2, "/api/", `^/api/v1$`, 0, "payments", nil, "rev-1", "100", "1", "user", "read", "api", "v1").
		AddRow("2", "10", "GET", 2, "/api/", `^/api/v2$`, 1, "payments", nil, "rev-2", "200", "2", "user", "read", "api", "v2").
		RowError(1, rowErr)

	mockDB.EXPECT().
		Query(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
			pool.ExpectQuery(".*").WillReturnRows(rows)
			return pool.Query(ctx, "test")
		})

	result, err := repo.FindCandidates(context.Background(), "GET", "/api/v1")
	if err == nil {
		t.Fatal("expected a rows error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on rows error, got %v", result)
	}
}

// brokenSqlizer implements sq.Sqlizer and always returns an error.
type brokenSqlizer struct{}

func (b brokenSqlizer) ToSql() (string, []interface{}, error) {
	return "", nil, errors.New("broken sqlizer")
}

func TestFindCandidates_BuildQueryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockDB := repository.NewMockDBClientInterface(ctrl)

	// Return a builder pre-loaded with a broken Where clause so that ToSql fails.
	mockDB.EXPECT().Builder().
		Return(sq.StatementBuilder.PlaceholderFormat(sq.Dollar).Where(brokenSqlizer{})).
		AnyTimes()

	repo := NewPostgresRuleRepository(mockDB, nil)

	result, err := repo.FindCandidates(context.Background(), "GET", "/test")
	if err == nil {
		t.Fatal("expected a build query error, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on build error, got %v", result)
	}
	if !strings.Contains(err.Error(), "failed to build rule candidates query") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestFindCandidates_SegmentCountAndPrefixesComputedFromPath(t *testing.T) {
	mockDB, pool := setupMocks(t)
	repo := NewPostgresRuleRepository(mockDB, nil)

	rows := pool.NewRows(ruleColumns)

	mockDB.EXPECT().
		Query(gomock.Any(), gomock.Any(), "PUT", []string{"/a/", "/a/b/", "/a/b/c"}, 3).
		DoAndReturn(func(ctx context.Context, query string, args ...interface{}) (interface{}, error) {
			pool.ExpectQuery(".*").WillReturnRows(rows)
			return pool.Query(ctx, "test")
		})

	_, err := repo.FindCandidates(context.Background(), "PUT", "/a/b/c")
	if err != nil {
		t.Fatalf("FindCandidates returned error: %v", err)
	}
}

// ── scanRulesWithTuples (additional edge cases) ──────────────────────────────

func TestScanRulesWithTuples_PreservesRuleOrder(t *testing.T) {
	pool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create pgxmock pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	rows := pool.NewRows(ruleColumns).
		AddRow("3", "10", "GET", 1, "/", `^/c$`, 0, "payments", nil, "rev-3", "300", "3", "user", "edit", "res", "c").
		AddRow("1", "10", "GET", 1, "/", `^/a$`, 1, "payments", nil, "rev-1", "100", "1", "user", "edit", "res", "a").
		AddRow("2", "10", "GET", 1, "/", `^/b$`, 2, "payments", nil, "rev-2", "200", "2", "user", "edit", "res", "b")

	pool.ExpectQuery("test").WillReturnRows(rows)
	pgxRows, err := pool.Query(context.Background(), "test")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer pgxRows.Close()

	result, err := scanRulesWithTuples(pgxRows)
	if err != nil {
		t.Fatalf("scanRulesWithTuples returned error: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(result))
	}
	// Order should be preserved as returned from query: 3, 1, 2
	expectedOrder := []string{"3", "1", "2"}
	for i, id := range expectedOrder {
		if result[i].Id != id {
			t.Errorf("result[%d].Id = %q, want %q", i, result[i].Id, id)
		}
	}
}
