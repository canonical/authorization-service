package rules

import (
    "context"

    "github.com/openfga/go-sdk/client"

    "github.com/canonical/authorization-service/internal/model/rules"
)

type RuleMatcherInterface interface {
    Match(candidates []*rules.RuleWithTuples, path string) (*rules.RuleWithTuples, error)
}

type TupleResolverInterface interface {
    Resolve(userID string, rule *rules.RuleWithTuples, match *rules.RegexMatch) ([]*rules.Tuple, error)
}

type ResourceMapperInterface interface {
    Map(ctx context.Context, userID, method, path string) ([]client.ClientBatchCheckItem, error)
}
