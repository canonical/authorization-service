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
func (r *PostgresRuleRepository) buildFindCandidatesQuery(method, path string, segmentCount int) (string, []interface{}, error) {
	return r.db.Builder().
		Select(
			"r.id", "r.service_id", "r.method", "r.segment_count", "r.static_prefix", "r.path_regex", "r.priority",
			"t.id", "t.rule_id", "t.user_resource_type", "t.permission", "t.object_resource_type", "t.object_resource_id",
		).
		From("authorization_rule r").
		Join("authorization_rule_tuple t ON t.rule_id = r.id").
		Where(sq.Eq{"r.method": method}).
		Where(sq.Expr("starts_with(?, r.static_prefix)", path)).
		Where(sq.LtOrEq{"r.segment_count": segmentCount}).
		OrderBy("r.priority ASC").
		ToSql()
}

// FindCandidates retrieves candidate rules (with their tuples) for the given HTTP method and path.
func (r *PostgresRuleRepository) FindCandidates(ctx context.Context, method, path string) ([]*rules.RuleWithTuples, error) {
	segmentCount := countSegments(path)

	query, args, err := r.buildFindCandidatesQuery(method, path, segmentCount)
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
