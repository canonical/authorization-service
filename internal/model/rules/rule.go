// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package rules

type Tuple struct {
	User         string
	Relation     string
	Object       string
	RuleTupleRef string
}
type RuleTuple struct {
	Id                 string `json:"id"`
	RuleId             string `json:"rule_id"`
	UserResourceType   string `json:"user_resource_type"`
	Permission         string `json:"permission"`
	ObjectResourceType string `json:"object_resource_type"`
	ObjectResourceId   string `json:"object_resource_id"`
}

type RuleWithTuples struct {
	Id           string       `json:"id"`
	ServiceId    string       `json:"service_id"`
	ServiceSlug  string       `json:"service_slug"`
	Method       string       `json:"method"`
	SegmentCount int          `json:"segment_count"`
	StaticPrefix string       `json:"static_prefix"`
	PathRegex    string       `json:"path_regex"`
	Priority     int          `json:"priority"`
	Tenant       *string      `json:"tenant"`
	Revision     string       `json:"revision"`
	RuleTuples   []*RuleTuple `json:"rule_tuples"`
}

// RegexMatches maps named capture group identifiers to their captured values.
type RegexMatches map[string]string
