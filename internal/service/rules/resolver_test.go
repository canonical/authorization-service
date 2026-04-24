//go:generate mockgen -build_flags=--mod=mod -package=mocks -destination=mocks/mock_repo.go github.com/canonical/authorization-service/internal/repository RuleRepository
//go:generate mockgen -build_flags=--mod=mod -package=mocks -destination=mocks/mock_rules.go . RuleMatcherInterface,TupleResolverInterface

package rules

import (
	"context"
	"errors"
	"testing"

	"github.com/openfga/go-sdk/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/canonical/authorization-service/internal/model/rules"
	"github.com/canonical/authorization-service/internal/service/rules/mocks"
)

func TestRuleMatcher_Match(t *testing.T) {
	candidates := []*rules.RuleWithTuples{
		{Id: "rule1", PathRegex: "^/users/([^/]+)$"},
		{Id: "rule2", PathRegex: "^/groups/([^/]+)$"},
		{Id: "rule3", PathRegex: "^/groups/([^/]+)/members$"},
	}

	testCases := []struct {
		name          string
		path          string
		candidates    []*rules.RuleWithTuples
		expectedRule  *rules.RuleWithTuples
		expectedError string
	}{
		{
			name:         "first rule matches",
			path:         "/users/123",
			candidates:   candidates,
			expectedRule: candidates[0],
		},
		{
			name:         "second rule matches",
			path:         "/groups/456",
			candidates:   candidates,
			expectedRule: candidates[1],
		},
		{
			name:         "third rule matches",
			path:         "/groups/456/members",
			candidates:   candidates,
			expectedRule: candidates[2],
		},
		{
			name:         "no rule matches",
			path:         "/other/path",
			candidates:   candidates,
			expectedRule: nil,
		},
		{
			name:          "invalid regex",
			path:          "/any",
			candidates:    []*rules.RuleWithTuples{{Id: "rule-invalid", PathRegex: "[invalid"}},
			expectedError: `invalid path regex "[invalid" for rule rule-invalid: error parsing regexp: missing closing ]: ` + "`" + `[invalid` + "`" + ``,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			matcher := NewRuleMatcher()
			matchedRule, err := matcher.Match(tc.candidates, tc.path)

			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Equal(t, tc.expectedError, err.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedRule, matchedRule)
			}
		})
	}
}

func TestTupleResolver_Resolve(t *testing.T) {
	ruleWithTuples := &rules.RuleWithTuples{
		Id: "rule1",
		RuleTuples: []*rules.RuleTuple{
			{Id: "tuple1", UserResourceType: "user", Permission: "view", ObjectResourceType: "group", ObjectResourceId: "{groupId}"},
		},
	}

	ruleWithMultipleTuples := &rules.RuleWithTuples{
		Id: "rule2",
		RuleTuples: []*rules.RuleTuple{
			{Id: "tuple1", UserResourceType: "user", Permission: "view", ObjectResourceType: "team", ObjectResourceId: "{teamId}"},
			{Id: "tuple2", UserResourceType: "user", Permission: "view", ObjectResourceType: "member", ObjectResourceId: "{memberId}"},
		},
	}

	testCases := []struct {
		name          string
		userID        string
		rule          *rules.RuleWithTuples
		matches       rules.RegexMatches
		expected      []*rules.Tuple
		expectedError string
	}{
		{
			name:    "successful resolution",
			userID:  "user123",
			rule:    ruleWithTuples,
			matches: rules.RegexMatches{"groupId": "group456"},
			expected: []*rules.Tuple{
				{User: "user:user123", Relation: "view", Object: "group:group456", RuleTupleRef: "0"},
			},
		},
		{
			name:   "successful resolution with multiple capture groups",
			userID: "user123",
			rule:   ruleWithMultipleTuples,
			matches: rules.RegexMatches{
				"teamId":   "team789",
				"memberId": "member012",
			},
			expected: []*rules.Tuple{
				{User: "user:user123", Relation: "view", Object: "team:team789", RuleTupleRef: "0"},
				{User: "user:user123", Relation: "view", Object: "member:member012", RuleTupleRef: "1"},
			},
		},
		{
			name:          "rule with no tuples",
			userID:        "user123",
			rule:          &rules.RuleWithTuples{Id: "rule-no-tuples"},
			matches:       nil,
			expectedError: "rule rule-no-tuples has no tuples",
		},
		{
			name:          "failed to resolve dynamic object ID",
			userID:        "user123",
			rule:          ruleWithTuples,
			matches:       rules.RegexMatches{"wrongId": "group456"},
			expectedError: `failed to resolve dynamic object resource ID for rule rule1: capture group "groupId" not found in regex matches`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resolver := NewTupleResolver()
			tuples, err := resolver.Resolve(tc.userID, tc.rule, tc.matches)

			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Equal(t, tc.expectedError, err.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, tuples)
			}
		})
	}
}

func TestExtractDynamicObjectID(t *testing.T) {
	testCases := []struct {
		name          string
		objectID      string
		matches       rules.RegexMatches
		expected      string
		expectedError string
	}{
		{
			name:     "static object ID",
			objectID: "static-id",
			matches:  nil,
			expected: "static-id",
		},
		{
			name:     "dynamic object ID with matching group",
			objectID: "{groupId}",
			matches:  rules.RegexMatches{"groupId": "group123"},
			expected: "group123",
		},
		{
			name:     "dynamic object ID picks correct group from multiple",
			objectID: "{memberId}",
			matches:  rules.RegexMatches{"teamId": "team1", "memberId": "member2"},
			expected: "member2",
		},
		{
			name:          "dynamic object ID with no matches",
			objectID:      "{groupId}",
			matches:       nil,
			expectedError: "rule requires a regex capture group but no regex matches were provided",
		},
		{
			name:          "dynamic object ID with missing group",
			objectID:      "{groupId}",
			matches:       rules.RegexMatches{"wrongId": "group123"},
			expectedError: `capture group "groupId" not found in regex matches`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := extractDynamicObjectID(tc.objectID, tc.matches)
			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Equal(t, tc.expectedError, err.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestResourceMapper_Map(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockRuleRepository(ctrl)
	mockMatcher := mocks.NewMockRuleMatcherInterface(ctrl)
	mockResolver := mocks.NewMockTupleResolverInterface(ctrl)

	ctx := context.Background()
	userID := "user123"
	method := "GET"
	path := "/groups/group456"

	candidates := []*rules.RuleWithTuples{{Id: "rule1"}}
	matchedRule := candidates[0]
	resolvedTuples := []*rules.Tuple{{User: "user:user123", Relation: "view", Object: "group:group456", RuleTupleRef: "0"}}

	testCases := []struct {
		name          string
		setupMocks    func()
		expected      []client.ClientBatchCheckItem
		expectedError string
	}{
		{
			name: "successful mapping",
			setupMocks: func() {
				mockRepo.EXPECT().FindCandidates(ctx, method, path).Return(candidates, nil)
				mockMatcher.EXPECT().Match(candidates, path).Return(matchedRule, nil)
				mockResolver.EXPECT().Resolve(userID, matchedRule, gomock.Any()).Return(resolvedTuples, nil)
			},
			expected: []client.ClientBatchCheckItem{
				{User: "user:user123", Relation: "view", Object: "group:group456", CorrelationId: "0"},
			},
		},
		{
			name: "FindCandidates fails",
			setupMocks: func() {
				mockRepo.EXPECT().FindCandidates(ctx, method, path).Return(nil, errors.New("db error"))
			},
			expectedError: "failed to find rule candidates: db error",
		},
		{
			name: "no candidates found",
			setupMocks: func() {
				mockRepo.EXPECT().FindCandidates(ctx, method, path).Return(nil, nil)
			},
			expected: nil,
		},
		{
			name: "Match fails",
			setupMocks: func() {
				mockRepo.EXPECT().FindCandidates(ctx, method, path).Return(candidates, nil)
				mockMatcher.EXPECT().Match(candidates, path).Return(nil, errors.New("match error"))
			},
			expectedError: "failed to match rule: match error",
		},
		{
			name: "no rule matched",
			setupMocks: func() {
				mockRepo.EXPECT().FindCandidates(ctx, method, path).Return(candidates, nil)
				mockMatcher.EXPECT().Match(candidates, path).Return(nil, nil)
			},
			expected: nil,
		},
		{
			name: "Resolve fails",
			setupMocks: func() {
				mockRepo.EXPECT().FindCandidates(ctx, method, path).Return(candidates, nil)
				mockMatcher.EXPECT().Match(candidates, path).Return(matchedRule, nil)
				mockResolver.EXPECT().Resolve(userID, matchedRule, gomock.Any()).Return(nil, errors.New("resolve error"))
			},
			expectedError: "failed to resolve tuples: resolve error",
		},
		{
			name: "extractRegexMatch fails",
			setupMocks: func() {
				mockRepo.EXPECT().FindCandidates(ctx, method, path).Return(candidates, nil)
				matchedRule.PathRegex = "[invalid"
				mockMatcher.EXPECT().Match(candidates, path).Return(matchedRule, nil)
			},
			expectedError: `failed to extract regex match: invalid path regex "[invalid": error parsing regexp: missing closing ]: ` + "`" + `[invalid` + "`" + ``,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset regex for other tests
			matchedRule.PathRegex = ""
			tc.setupMocks()
			mapper := NewResourceMapper(mockRepo, mockMatcher, mockResolver)
			result, err := mapper.Map(ctx, userID, method, path)

			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Equal(t, tc.expectedError, err.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestExtractRegexMatches(t *testing.T) {
	testCases := []struct {
		name          string
		pathRegex     string
		path          string
		expected      rules.RegexMatches
		expectedError string
	}{
		{
			name:      "regex with named capture group",
			pathRegex: `^/groups/(?P<groupId>[^/]+)$`,
			path:      "/groups/group123",
			expected:  rules.RegexMatches{"groupId": "group123"},
		},
		{
			name:      "regex with multiple named capture groups",
			pathRegex: `^/api/teams/(?P<teamId>[^/]+)/members/(?P<memberId>[^/]+)$`,
			path:      "/api/teams/team1/members/member2",
			expected:  rules.RegexMatches{"teamId": "team1", "memberId": "member2"},
		},
		{
			name:      "regex with no named capture group",
			pathRegex: `^/groups/([^/]+)$`,
			path:      "/groups/group123",
			expected:  nil,
		},
		{
			name:      "no match",
			pathRegex: `^/users/(?P<userId>[^/]+)$`,
			path:      "/groups/group123",
			expected:  nil,
		},
		{
			name:          "invalid regex",
			pathRegex:     "[invalid",
			path:          "/any",
			expectedError: `invalid path regex "[invalid": error parsing regexp: missing closing ]: ` + "`" + `[invalid` + "`" + ``,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := extractRegexMatches(tc.pathRegex, tc.path)
			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Equal(t, tc.expectedError, err.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}
