package repository

import (
	"context"
	"fmt"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	"github.com/canonical/authorization-service/internal/integration/postgres"
	"github.com/canonical/authorization-service/internal/model/rules"
)

type RuleRepository interface {
	FindCandidates(ctx context.Context, method, path string) ([]*rules.RuleWithTuples, error)
}

// Compile-time check to ensure PostgresRuleRepository implements RuleRepository
var _ RuleRepository = (*PostgresRuleRepository)(nil)

// PostgresRuleRepository implements RuleRepository using PostgreSQL.
type PostgresRuleRepository struct {
	db postgres.DBClientInterface
}

// NewPostgresRuleRepository creates a new PostgresRuleRepository.
func NewPostgresRuleRepository(db postgres.DBClientInterface) *PostgresRuleRepository {
	return &PostgresRuleRepository{db: db}
}

// buildFindCandidatesQuery builds the SQL query for finding candidate rules using Squirrel.
func (r *PostgresRuleRepository) buildFindCandidatesQuery(method string, pathPrefixes []string, segmentCount int) (string, []interface{}, error) {
	return r.db.Builder().
		Select(
			"r.id", "r.service_id", "r.method", "r.segment_count", "r.static_prefix", "r.path_regex", "r.priority",
			"t.id", "t.rule_id", "t.user_resource_type", "t.permission", "t.object_resource_type", "t.object_resource_id",
		).
		From("authorization_rule r").
		Join("authorization_rule_tuple t ON t.rule_id = r.id").
		Where(sq.Eq{"r.method": method}).
		Where(sq.Expr("r.static_prefix = ANY(?)", pathPrefixes)).
		Where(sq.LtOrEq{"r.segment_count": segmentCount}).
		OrderBy("r.segment_count DESC").
		OrderBy("r.priority ASC").
		ToSql()
}

// FindCandidates retrieves candidate rules (with their tuples) for the given HTTP method and path.
func (r *PostgresRuleRepository) FindCandidates(ctx context.Context, method, path string) ([]*rules.RuleWithTuples, error) {
	if method == "" || path == "" {
		return nil, fmt.Errorf("method and path cannot be empty")
	}

	segmentCount := countSegments(path)
	pathPrefixes := buildPrefixes(path, segmentCount)

	query, args, err := r.buildFindCandidatesQuery(method, pathPrefixes, segmentCount)
	if err != nil {
		return nil, fmt.Errorf("failed to build rule candidates query: %w", err)
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query rule candidates: %w", err)
	}
	defer rows.Close()

	return scanRulesWithTuples(rows)
}

// buildPrefixes returns all cumulative prefixes of a path.
//
// Every intermediate segment boundary is represented with a trailing slash,
// matching how static_prefix values are stored in the database when the next
// segment is dynamic (e.g. "/api/v1/groups/" for a rule like
// "/api/v1/groups/{groupID}/member").
// The final entry is the full path as-is: with a trailing slash if the caller
// supplied one, without otherwise.
//
// Example
//
//	/api/v1/groups/group-id-1 -> ["/api/", "/api/v1/", "/api/v1/groups/", "/api/v1/groups/group-id-1"]
func buildPrefixes(path string, segments int) []string {
	if path == "/" {
		return []string{"/"}
	}

	// avoid reallocations by using a number that will be 100% sufficient
	prefixes := make([]string, 0, segments+1)

	start := 0
	if path[0] == '/' {
		start = 1
	}

	for i := start; i < len(path); i++ {
		if path[i] == '/' {
			prefixes = append(prefixes, path[:i+1]) // intermediate prefix WITH trailing slash
		}
	}

	// Append the full path only when it is not already covered by the last
	// intermediate prefix (i.e. the original path did not end with a slash).
	if path[len(path)-1] != '/' {
		prefixes = append(prefixes, path)
	}

	return prefixes
}

// countSegments counts the number of non-empty path segments in a URL path.
// e.g. "/api/v1/groups/123" → 4
func countSegments(path string) int {
	return len(strings.FieldsFunc(path, func(r rune) bool { return r == '/' }))
}

// scanRulesWithTuples scans rows from the candidates query and groups tuples
// by their parent rule. Each row is a join of authorization_rule + authorization_rule_tuple.
func scanRulesWithTuples(rows pgx.Rows) ([]*rules.RuleWithTuples, error) {
	// Use an ordered map to preserve priority order and group tuples per rule.
	ruleIndex := make(map[string]*rules.RuleWithTuples)
	var ruleOrder []string

	for rows.Next() {
		var (
			ruleID       string
			serviceID    string
			method       string
			segmentCount int
			staticPrefix string
			pathRegex    string
			priority     int

			tupleID            string
			tupleRuleID        string
			userResourceType   string
			permission         string
			objectResourceType string
			objectResourceID   string
		)

		if err := rows.Scan(
			&ruleID,
			&serviceID,
			&method,
			&segmentCount,
			&staticPrefix,
			&pathRegex,
			&priority,
			&tupleID,
			&tupleRuleID,
			&userResourceType,
			&permission,
			&objectResourceType,
			&objectResourceID,
		); err != nil {
			return nil, fmt.Errorf("failed to scan rule candidate row: %w", err)
		}

		r, exists := ruleIndex[ruleID]
		if !exists {
			r = &rules.RuleWithTuples{
				Id:           ruleID,
				ServiceId:    serviceID,
				Method:       method,
				SegmentCount: segmentCount,
				StaticPrefix: staticPrefix,
				PathRegex:    pathRegex,
				Priority:     priority,
			}
			ruleIndex[ruleID] = r
			ruleOrder = append(ruleOrder, ruleID)
		}

		r.RuleTuples = append(r.RuleTuples, &rules.RuleTuple{
			Id:                 tupleID,
			RuleId:             tupleRuleID,
			UserResourceType:   userResourceType,
			Permission:         permission,
			ObjectResourceType: objectResourceType,
			ObjectResourceId:   objectResourceID,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rule candidate rows: %w", err)
	}

	result := make([]*rules.RuleWithTuples, 0, len(ruleOrder))
	for _, id := range ruleOrder {
		result = append(result, ruleIndex[id])
	}
	return result, nil
}
