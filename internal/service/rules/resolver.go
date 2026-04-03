package rules

import (
    "context"
    "fmt"
    "regexp"
    "strings"

    "github.com/openfga/go-sdk/client"

    "github.com/canonical/authorization-service/internal/model/rules"
    "github.com/canonical/authorization-service/internal/repository"
)

// Compile-time checks
var (
    _ RuleMatcherInterface    = (*RuleMatcher)(nil)
    _ TupleResolverInterface  = (*TupleResolver)(nil)
    _ ResourceMapperInterface = (*ResourceMapper)(nil)
)

// RuleMatcher matches an incoming path against a set of candidate rules.
type RuleMatcher struct{}

// NewRuleMatcher creates a new RuleMatcher.
func NewRuleMatcher() *RuleMatcher {
    return &RuleMatcher{}
}

// Match iterates over candidates (already sorted by priority ASC from the query)
// and returns the first rule whose path_regex matches the given path.
func (m *RuleMatcher) Match(candidates []*rules.RuleWithTuples, path string) (*rules.RuleWithTuples, error) {
    for _, candidate := range candidates {
        re, err := regexp.Compile(candidate.PathRegex)
        if err != nil {
            return nil, fmt.Errorf("invalid path regex %q for rule %s: %w", candidate.PathRegex, candidate.Id, err)
        }

        if re.MatchString(path) {
            return candidate, nil
        }
    }

    return nil, nil
}

// TupleResolver resolves a RuleWithTuples + regex match into an OpenFGA Tuple.
type TupleResolver struct{}

// NewTupleResolver creates a new TupleResolver.
func NewTupleResolver() *TupleResolver {
    return &TupleResolver{}
}

// Resolve builds an openfga Tuple from the rule tuple specifications, substituting
// dynamic object_resource_id values (wrapped in braces like "{groupId}") with
// the corresponding capture group value from the regex match.
func (r *TupleResolver) Resolve(userID string, rule *rules.RuleWithTuples, match *rules.RegexMatch) ([]*rules.Tuple, error) {
    if len(rule.RuleTuples) == 0 {
        return nil, fmt.Errorf("rule %s has no tuples", rule.Id)
    }

    tuples := make([]*rules.Tuple, 0, len(rule.RuleTuples))
    for idx, spec := range rule.RuleTuples {
        objectID, err := extractDynamicObjectID(spec.ObjectResourceId, match)
        if err != nil {
            return nil, fmt.Errorf("failed to resolve dynamic object resource ID for rule %s: %w", rule.Id, err)
        }

        user := fmt.Sprintf("%s:%s", spec.UserResourceType, userID)
        object := fmt.Sprintf("%s:%s", spec.ObjectResourceType, objectID)
        ruleTupleRef := fmt.Sprintf("%d", idx)

        tuples = append(tuples, &rules.Tuple{
            User:         user,
            Relation:     spec.Permission,
            Object:       object,
            RuleTupleRef: ruleTupleRef,
        })
    }

    return tuples, nil
}

func extractDynamicObjectID(objectID string, match *rules.RegexMatch) (string, error) {
    if !(strings.HasPrefix(objectID, "{") && strings.HasSuffix(objectID, "}")) {
        return objectID, nil
    }

    if match == nil {
        return "", fmt.Errorf("rule requires a regex capture group but no regex match was provided")
    }

    // unwrap curly braces
    groupName := objectID[1 : len(objectID)-1]
    if match.Identifier != groupName {
        return "", fmt.Errorf("expected capture group %q but got %q", groupName, match.Identifier)
    }

    return match.Value, nil
}

// ResourceMapper implements the full pipeline: lookup → match → resolve.
type ResourceMapper struct {
    repo     repository.RuleRepository
    matcher  RuleMatcherInterface
    resolver TupleResolverInterface
}

// NewResourceMapper creates a new ResourceMapper.
func NewResourceMapper(repo repository.RuleRepository, matcher RuleMatcherInterface, resolver TupleResolverInterface) *ResourceMapper {
    return &ResourceMapper{
        repo:     repo,
        matcher:  matcher,
        resolver: resolver,
    }
}

// Map takes an HTTP method and path, looks up candidate rules from the database,
// matches the first applicable rule, resolves its tuple, and returns the result.
func (rm *ResourceMapper) Map(ctx context.Context, userID, method, path string) ([]client.ClientBatchCheckItem, error) {
    candidates, err := rm.repo.FindCandidates(ctx, method, path)
    if err != nil {
        return nil, fmt.Errorf("failed to find rule candidates: %w", err)
    }

    if len(candidates) == 0 {
        return nil, nil
    }

    matchedRule, err := rm.matcher.Match(candidates, path)
    if err != nil {
        return nil, fmt.Errorf("failed to match rule: %w", err)
    }

    if matchedRule == nil {
        return nil, nil
    }

    // Extract named capture groups from the path using the matched rule's regex.
    regexMatch, err := extractRegexMatch(matchedRule.PathRegex, path)
    if err != nil {
        return nil, fmt.Errorf("failed to extract regex match: %w", err)
    }

    tuples, err := rm.resolver.Resolve(userID, matchedRule, regexMatch)
    if err != nil {
        return nil, fmt.Errorf("failed to resolve tuples: %w", err)
    }

    batchReqItems := make([]client.ClientBatchCheckItem, 0, len(tuples))
    for _, tuple := range tuples {
        batchReqItems = append(batchReqItems, client.ClientBatchCheckItem{
            User:          tuple.User,
            Relation:      tuple.Relation,
            Object:        tuple.Object,
            CorrelationId: tuple.RuleTupleRef,
        })
    }

    return batchReqItems, nil
}

// extractRegexMatch compiles the path regex and extracts the first named capture group
// from the path. Returns nil if there are no named groups.
func extractRegexMatch(pathRegex, path string) (*rules.RegexMatch, error) {
    re, err := regexp.Compile(pathRegex)
    if err != nil {
        return nil, fmt.Errorf("invalid path regex %q: %w", pathRegex, err)
    }

    names := re.SubexpNames()
    match := re.FindStringSubmatch(path)
    if match == nil {
        return nil, nil
    }

    for i, name := range names {
        if i > 0 && name != "" && i < len(match) {
            return &rules.RegexMatch{
                Identifier: name,
                Value:      match[i],
            }, nil
        }
    }

    return nil, nil
}
