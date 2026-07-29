// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package rules

import "gopkg.in/yaml.v3"

type SeedFile struct {
	Version     string     `yaml:"version"`
	Service     string     `yaml:"service"`
	Revision    string     `yaml:"revision"`
	Description string     `yaml:"description"`
	Rules       []SeedRule `yaml:"rules"`
	FilePath    string     `yaml:"-"`
}

type SeedRule struct {
	Method   string      `yaml:"method"`
	Match    string      `yaml:"match"`
	Priority int         `yaml:"priority"`
	Tuples   []SeedTuple `yaml:"tuples"`
}

type SeedTuple struct {
	UserResourceType   string `yaml:"userResourceType"`
	Permission         string `yaml:"permission"`
	ObjectResourceType string `yaml:"objectResourceType"`
	ObjectResourceId   string `yaml:"objectResourceId"`
}

type CompiledRuleWithTuples struct {
	Method       string
	SegmentCount int
	StaticPrefix string
	PathRegex    string
	Priority     int
	Tuples       []CompiledRuleTuple
}

type CompiledRuleTuple struct {
	UserResourceType   string
	Permission         string
	ObjectResourceType string
	ObjectResourceId   string
}

// UnmarshalYAML implements custom unmarshaling to support both camelCase and snake_case keys in YAML files.
func (t *SeedTuple) UnmarshalYAML(value *yaml.Node) error {
	type rawTuple struct {
		UserResourceType    string `yaml:"userResourceType"`
		UserResourceType_   string `yaml:"user_resource_type"`
		Permission          string `yaml:"permission"`
		ObjectResourceType  string `yaml:"objectResourceType"`
		ObjectResourceType_ string `yaml:"object_resource_type"`
		ObjectResourceId    string `yaml:"objectResourceId"`
		ObjectResourceId_   string `yaml:"object_resource_id"`
	}
	var raw rawTuple
	if err := value.Decode(&raw); err != nil {
		return err
	}

	t.Permission = raw.Permission

	if raw.UserResourceType != "" {
		t.UserResourceType = raw.UserResourceType
	} else {
		t.UserResourceType = raw.UserResourceType_
	}

	if raw.ObjectResourceType != "" {
		t.ObjectResourceType = raw.ObjectResourceType
	} else {
		t.ObjectResourceType = raw.ObjectResourceType_
	}

	if raw.ObjectResourceId != "" {
		t.ObjectResourceId = raw.ObjectResourceId
	} else {
		t.ObjectResourceId = raw.ObjectResourceId_
	}

	return nil
}
