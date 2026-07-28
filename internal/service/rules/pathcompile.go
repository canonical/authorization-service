// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package rules

import (
	"fmt"
	"regexp"
	"strings"
)

type CompiledPath struct {
	SegmentCount int
	StaticPrefix string
	PathRegex    string
}

// CompileMatch compiles a match path pattern into a CompiledPath structure.
func CompileMatch(match string) (CompiledPath, error) {
	if match == "" {
		return CompiledPath{}, fmt.Errorf("match path cannot be empty")
	}
	if !strings.HasPrefix(match, "/") {
		return CompiledPath{}, fmt.Errorf("match path must start with '/'")
	}

	// Calculate segment count
	segmentCount := len(strings.FieldsFunc(match, func(r rune) bool { return r == '/' }))

	// Calculate static prefix ending right before the first dynamic construct ({placeholder} or **)
	staticPrefix := getStaticPrefix(match)

	// Build path regex
	// We replace {placeholder} with (?P<placeholder>[^/]+) and wildcard ** with .* and escape regex characters in static parts.
	reConstruct := regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}|\*\*`)

	// Find all dynamic constructs
	matches := reConstruct.FindAllStringSubmatchIndex(match, -1)

	var sb strings.Builder
	sb.WriteString("^")

	lastIdx := 0
	for _, loc := range matches {
		start, end := loc[0], loc[1]

		// Escape the static part before this construct
		sb.WriteString(regexp.QuoteMeta(match[lastIdx:start]))

		if loc[2] != -1 {
			// It matched a placeholder {name}
			paramName := match[loc[2]:loc[3]]
			sb.WriteString(fmt.Sprintf("(?P<%s>[^/]+)", paramName))
		} else {
			// It matched wildcard **
			sb.WriteString(".*")
		}

		lastIdx = end
	}
	// Escape any trailing static part
	sb.WriteString(regexp.QuoteMeta(match[lastIdx:]))
	sb.WriteString("$")

	return CompiledPath{
		SegmentCount: segmentCount,
		StaticPrefix: staticPrefix,
		PathRegex:    sb.String(),
	}, nil
}

func getStaticPrefix(match string) string {
	idxCurly := strings.Index(match, "{")
	idxWildcard := strings.Index(match, "**")

	firstIdx := -1
	if idxCurly != -1 && idxWildcard != -1 {
		if idxCurly < idxWildcard {
			firstIdx = idxCurly
		} else {
			firstIdx = idxWildcard
		}
	} else if idxCurly != -1 {
		firstIdx = idxCurly
	} else if idxWildcard != -1 {
		firstIdx = idxWildcard
	}

	if firstIdx == -1 {
		return match
	}
	return match[:firstIdx]
}
