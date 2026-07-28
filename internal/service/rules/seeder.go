// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package rules

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/canonical/authorization-service/internal/integration/postgres"
	"github.com/canonical/authorization-service/internal/model/rules"
	"github.com/canonical/authorization-service/internal/repository"
)

type RuleSeeder struct {
	db   postgres.DBClientInterface
	repo *repository.PostgresRuleRepository
}

func NewRuleSeeder(db postgres.DBClientInterface, repo *repository.PostgresRuleRepository) *RuleSeeder {
	return &RuleSeeder{
		db:   db,
		repo: repo,
	}
}

// LoadSeedFiles recursively scans the root directory for any rules.yaml files.
func LoadSeedFiles(root string) ([]rules.SeedFile, error) {
	var files []rules.SeedFile
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "rules.yaml" {
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read seed file %s: %w", path, err)
			}
			var sf rules.SeedFile
			if err := yaml.Unmarshal(content, &sf); err != nil {
				return fmt.Errorf("failed to parse YAML in %s: %w", path, err)
			}
			sf.FilePath = path
			files = append(files, sf)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// ValidateSeedFile validates the content of a SeedFile against structural requirements.
func ValidateSeedFile(f rules.SeedFile) error {
	if f.Version != "1" {
		return fmt.Errorf("invalid version %q (must be \"1\") in file %s", f.Version, f.FilePath)
	}
	if f.Service == "" {
		return fmt.Errorf("service slug cannot be empty in file %s", f.FilePath)
	}
	if f.Revision == "" {
		return fmt.Errorf("revision cannot be empty in file %s", f.FilePath)
	}
	if len(f.Rules) == 0 {
		return fmt.Errorf("file %s must contain at least one rule", f.FilePath)
	}

	seenRules := make(map[string]bool)
	validMethods := map[string]bool{
		"GET":     true,
		"POST":    true,
		"PUT":     true,
		"PATCH":   true,
		"DELETE":  true,
		"HEAD":    true,
		"OPTIONS": true,
	}

	for idx, r := range f.Rules {
		if !validMethods[r.Method] {
			return fmt.Errorf("invalid HTTP method %q at rule %d in file %s", r.Method, idx, f.FilePath)
		}
		if r.Match == "" {
			return fmt.Errorf("match path cannot be empty at rule %d in file %s", idx, f.FilePath)
		}
		if !strings.HasPrefix(r.Match, "/") {
			return fmt.Errorf("match path %q at rule %d must start with '/' in file %s", r.Match, idx, f.FilePath)
		}
		if len(r.Tuples) == 0 {
			return fmt.Errorf("rule %d (%s %s) in file %s must contain at least one tuple", idx, r.Method, r.Match, f.FilePath)
		}

		// Check well-formed and balanced placeholders
		if err := checkWellFormedPlaceholders(r.Match); err != nil {
			return fmt.Errorf("rule match %q is invalid: %w", r.Match, err)
		}

		// Check duplicate rules (method + match)
		ruleKey := fmt.Sprintf("%s:%s", r.Method, r.Match)
		if seenRules[ruleKey] {
			return fmt.Errorf("duplicate rule %s at rule %d in file %s", ruleKey, idx, f.FilePath)
		}
		seenRules[ruleKey] = true

		// Check wildcard position (must only appear at the end of the path)
		if strings.Contains(r.Match, "**") {
			if !strings.HasSuffix(r.Match, "**") {
				return fmt.Errorf("rule match %q is invalid: wildcard '**' must be terminal", r.Match)
			}
		}

		// Find path placeholders and check duplicates
		rePlaceholder := regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)
		pathPlaceholders := make(map[string]bool)
		pathPlaceholderMatches := rePlaceholder.FindAllStringSubmatch(r.Match, -1)
		for _, m := range pathPlaceholderMatches {
			name := m[1]
			if pathPlaceholders[name] {
				return fmt.Errorf("rule match %q is invalid: duplicate placeholder name %q", r.Match, name)
			}
			pathPlaceholders[name] = true
		}

		// Check duplicate tuples in the same rule
		type tupleKey struct {
			userType string
			perm     string
			objType  string
			objId    string
		}
		seenTuples := make(map[tupleKey]bool)

		for tIdx, t := range r.Tuples {
			if t.UserResourceType == "" {
				return fmt.Errorf("userResourceType cannot be empty at rule %d, tuple %d in file %s", idx, tIdx, f.FilePath)
			}
			if t.Permission == "" {
				return fmt.Errorf("permission cannot be empty at rule %d, tuple %d in file %s", idx, tIdx, f.FilePath)
			}
			if t.ObjectResourceType == "" {
				return fmt.Errorf("objectResourceType cannot be empty at rule %d, tuple %d in file %s", idx, tIdx, f.FilePath)
			}
			if t.ObjectResourceId == "" {
				return fmt.Errorf("objectResourceId cannot be empty at rule %d, tuple %d in file %s", idx, tIdx, f.FilePath)
			}

			tKey := tupleKey{
				userType: t.UserResourceType,
				perm:     t.Permission,
				objType:  t.ObjectResourceType,
				objId:    t.ObjectResourceId,
			}
			if seenTuples[tKey] {
				return fmt.Errorf("duplicate tuple %v at rule %d, tuple %d in file %s", tKey, idx, tIdx, f.FilePath)
			}
			seenTuples[tKey] = true

			paramName, isDynamic := classifyObjectID(t.ObjectResourceId)

			// Check wildcard with no other identifiers constraint
			if strings.Contains(r.Match, "**") && len(pathPlaceholders) == 0 {
				if isDynamic {
					return fmt.Errorf("tuple objectResourceId %q is invalid for match %q: dynamic objectResourceId requires at least one named placeholder in the path", t.ObjectResourceId, r.Match)
				}
			}

			// If placeholder is used, verify it exists in path
			if isDynamic {
				if !pathPlaceholders[paramName] {
					return fmt.Errorf("tuple objectResourceId %q is invalid for match %q: no such capture group", t.ObjectResourceId, r.Match)
				}
			}
		}
	}

	return nil
}

func checkWellFormedPlaceholders(match string) error {
	inBrace := false
	var currentName strings.Builder
	for i, r := range match {
		if r == '{' {
			if inBrace {
				return fmt.Errorf("nested curly braces are not allowed at index %d", i)
			}
			inBrace = true
			currentName.Reset()
		} else if r == '}' {
			if !inBrace {
				return fmt.Errorf("unmatched closing curly brace at index %d", i)
			}
			inBrace = false
			name := currentName.String()
			if name == "" {
				return fmt.Errorf("empty placeholder name is not allowed")
			}
			for _, ch := range name {
				if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_') {
					return fmt.Errorf("invalid character %q in placeholder name %q", ch, name)
				}
			}
		} else {
			if inBrace {
				currentName.WriteRune(r)
			}
		}
	}
	if inBrace {
		return fmt.Errorf("unclosed curly brace at the end of path")
	}
	return nil
}

func classifyObjectID(id string) (string, bool) {
	if strings.HasPrefix(id, "{") && strings.HasSuffix(id, "}") {
		return id[1 : len(id)-1], true
	}
	return id, false
}

// SeedService runs the seeding process for a single service SeedFile.
func (s *RuleSeeder) SeedService(ctx context.Context, file rules.SeedFile) (inserted bool, skipped bool, err error) {
	if err := ValidateSeedFile(file); err != nil {
		return false, false, fmt.Errorf("validation failed: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Upsert federated_service
	serviceID, err := s.repo.UpsertFederatedService(ctx, tx, file.Service, file.Description)
	if err != nil {
		return false, false, fmt.Errorf("failed to upsert service: %w", err)
	}

	// 2. Read current revision
	dbRevision, found, err := s.repo.GetCurrentRevision(ctx, tx, serviceID)
	if err != nil {
		return false, false, fmt.Errorf("failed to get current revision: %w", err)
	}

	// 3. Compare revisions
	if found {
		cmp := CompareRevision(file.Revision, dbRevision)
		if cmp <= 0 {
			// YAML revision is less than or equal to DB revision. Skip!
			return false, true, nil
		}
	}

	// 4. Compile rules
	var compiledRules []rules.CompiledRuleWithTuples
	for _, r := range file.Rules {
		cp, err := CompileMatch(r.Match)
		if err != nil {
			return false, false, fmt.Errorf("failed to compile match %q: %w", r.Match, err)
		}

		var tuples []rules.CompiledRuleTuple
		for _, t := range r.Tuples {
			paramName, isDynamic := classifyObjectID(t.ObjectResourceId)
			dbObjID := t.ObjectResourceId
			if isDynamic {
				dbObjID = "{" + paramName + "}"
			}

			tuples = append(tuples, rules.CompiledRuleTuple{
				UserResourceType:   t.UserResourceType,
				Permission:         t.Permission,
				ObjectResourceType: t.ObjectResourceType,
				ObjectResourceId:   dbObjID,
			})
		}

		compiledRules = append(compiledRules, rules.CompiledRuleWithTuples{
			Method:       r.Method,
			SegmentCount: cp.SegmentCount,
			StaticPrefix: cp.StaticPrefix,
			PathRegex:    cp.PathRegex,
			Priority:     r.Priority,
			Tuples:       tuples,
		})
	}

	// 5. Replace rules and tuples
	err = s.repo.ReplaceServiceRules(ctx, tx, serviceID, file.Revision, compiledRules)
	if err != nil {
		return false, false, fmt.Errorf("failed to replace service rules: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return false, false, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return true, false, nil
}

type revisionTokenKind int

const (
	tokenLetters revisionTokenKind = iota
	tokenDigits
)

type revisionToken struct {
	Kind revisionTokenKind
	Text string
}

// TokenizeRevision splits a revision string into its constituent numeric or letter tokens,
// treating any non-alphanumeric character as a boundary separator.
func TokenizeRevision(s string) []revisionToken {
	var tokens []revisionToken
	var current []rune
	var currentKind revisionTokenKind
	inToken := false

	for _, r := range s {
		var isDigit, isLetter bool
		if r >= '0' && r <= '9' {
			isDigit = true
		} else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			isLetter = true
		}

		if isDigit {
			if inToken && currentKind == tokenDigits {
				current = append(current, r)
			} else {
				if inToken {
					tokens = append(tokens, revisionToken{
						Kind: currentKind,
						Text: string(current),
					})
				}
				current = []rune{r}
				currentKind = tokenDigits
				inToken = true
			}
		} else if isLetter {
			if inToken && currentKind == tokenLetters {
				current = append(current, r)
			} else {
				if inToken {
					tokens = append(tokens, revisionToken{
						Kind: currentKind,
						Text: string(current),
					})
				}
				current = []rune{r}
				currentKind = tokenLetters
				inToken = true
			}
		} else {
			// Separator boundary character
			if inToken {
				tokens = append(tokens, revisionToken{
					Kind: currentKind,
					Text: string(current),
				})
				inToken = false
				current = nil
			}
		}
	}

	if inToken {
		tokens = append(tokens, revisionToken{
			Kind: currentKind,
			Text: string(current),
		})
	}

	return tokens
}

func compareDigits(da, db string) int {
	trimmedA := strings.TrimLeft(da, "0")
	trimmedB := strings.TrimLeft(db, "0")
	if trimmedA == "" {
		trimmedA = "0"
	}
	if trimmedB == "" {
		trimmedB = "0"
	}
	if len(trimmedA) < len(trimmedB) {
		return -1
	}
	if len(trimmedA) > len(trimmedB) {
		return 1
	}
	return strings.Compare(trimmedA, trimmedB)
}

func compareLetters(la, lb string) int {
	return strings.Compare(strings.ToLower(la), strings.ToLower(lb))
}

func isRemainingAllZeroes(tokens []revisionToken) bool {
	for _, t := range tokens {
		if t.Kind == tokenLetters {
			return false
		}
		if strings.TrimLeft(t.Text, "0") != "" {
			return false
		}
	}
	return true
}

// CompareRevision compares two revision strings using a natural-order comparator.
// Returns -1 if a < b, 0 if a == b, and 1 if a > b.
func CompareRevision(a, b string) int {
	tokensA := TokenizeRevision(a)
	tokensB := TokenizeRevision(b)

	minLen := len(tokensA)
	if len(tokensB) < minLen {
		minLen = len(tokensB)
	}

	for i := 0; i < minLen; i++ {
		ta := tokensA[i]
		tb := tokensB[i]

		if ta.Kind == tokenDigits && tb.Kind == tokenDigits {
			cmp := compareDigits(ta.Text, tb.Text)
			if cmp != 0 {
				return cmp
			}
		} else if ta.Kind == tokenLetters && tb.Kind == tokenLetters {
			cmp := compareLetters(ta.Text, tb.Text)
			if cmp != 0 {
				return cmp
			}
		} else {
			// letter token < numeric token
			if ta.Kind == tokenLetters {
				return -1
			} else {
				return 1
			}
		}
	}

	if len(tokensA) > len(tokensB) {
		if isRemainingAllZeroes(tokensA[minLen:]) {
			return 0
		}
		return 1
	} else if len(tokensB) > len(tokensA) {
		if isRemainingAllZeroes(tokensB[minLen:]) {
			return 0
		}
		return -1
	}

	return 0
}
