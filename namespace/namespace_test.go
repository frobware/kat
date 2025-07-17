package namespace

import (
	"slices"
	"testing"
)

func TestNewPattern(t *testing.T) {
	tests := []struct {
		name         string
		pattern      string
		expectErr    bool
		expectIsGlob bool
	}{
		{
			name:         "literal pattern",
			pattern:      "default",
			expectErr:    false,
			expectIsGlob: false,
		},
		{
			name:         "glob pattern with asterisk",
			pattern:      "go-*",
			expectErr:    false,
			expectIsGlob: true,
		},
		{
			name:         "glob pattern with question mark",
			pattern:      "test-?",
			expectErr:    false,
			expectIsGlob: true,
		},
		{
			name:         "glob pattern with brackets",
			pattern:      "test-[abc]",
			expectErr:    false,
			expectIsGlob: true,
		},
		{
			name:         "empty pattern",
			pattern:      "",
			expectErr:    true,
			expectIsGlob: false,
		},
		{
			name:         "invalid glob pattern",
			pattern:      "[invalid",
			expectErr:    true,
			expectIsGlob: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, err := newPattern(tt.pattern)

			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if pattern.isGlob != tt.expectIsGlob {
				t.Errorf("expected isGlob=%v, got %v", tt.expectIsGlob, pattern.isGlob)
			}

			if pattern.String() != tt.pattern {
				t.Errorf("expected String()=%q, got %q", tt.pattern, pattern.String())
			}
		})
	}
}

func TestPattern_Match(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		namespace   string
		expectMatch bool
	}{
		{
			name:        "literal match",
			pattern:     "default",
			namespace:   "default",
			expectMatch: true,
		},
		{
			name:        "literal no match",
			pattern:     "default",
			namespace:   "kube-system",
			expectMatch: false,
		},
		{
			name:        "glob asterisk match",
			pattern:     "go-*",
			namespace:   "go-service",
			expectMatch: true,
		},
		{
			name:        "glob asterisk no match",
			pattern:     "go-*",
			namespace:   "backend-service",
			expectMatch: false,
		},
		{
			name:        "glob question mark match",
			pattern:     "test-?",
			namespace:   "test-1",
			expectMatch: true,
		},
		{
			name:        "glob question mark no match",
			pattern:     "test-?",
			namespace:   "test-10",
			expectMatch: false,
		},
		{
			name:        "glob brackets match",
			pattern:     "test-[abc]",
			namespace:   "test-a",
			expectMatch: true,
		},
		{
			name:        "glob brackets no match",
			pattern:     "test-[abc]",
			namespace:   "test-d",
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, err := newPattern(tt.pattern)
			if err != nil {
				t.Fatalf("failed to create pattern: %v", err)
			}

			match := pattern.match(tt.namespace)
			if match != tt.expectMatch {
				t.Errorf("expected match=%v, got %v", tt.expectMatch, match)
			}
		})
	}
}

func TestParsePatterns(t *testing.T) {
	tests := []struct {
		name        string
		patterns    []string
		expectErr   bool
		expectCount int
	}{
		{
			name:        "empty patterns",
			patterns:    []string{},
			expectErr:   false,
			expectCount: 0,
		},
		{
			name:        "nil patterns",
			patterns:    nil,
			expectErr:   false,
			expectCount: 0,
		},
		{
			name:        "valid patterns",
			patterns:    []string{"default", "go-*", "test-?"},
			expectErr:   false,
			expectCount: 3,
		},
		{
			name:        "invalid pattern in list",
			patterns:    []string{"default", "[invalid", "go-*"},
			expectErr:   true,
			expectCount: 0,
		},
		{
			name:        "empty pattern in list",
			patterns:    []string{"default", "", "go-*"},
			expectErr:   true,
			expectCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patterns, err := ParsePatterns(tt.patterns)

			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(patterns) != tt.expectCount {
				t.Errorf("expected %d patterns, got %d", tt.expectCount, len(patterns))
			}
		})
	}
}

// Integration test combining multiple operations.
func TestIntegration_PatternWorkflow(t *testing.T) {
	includePatterns, err := ParsePatterns([]string{"frontend-*", "backend-*", "bpfman"})
	if err != nil {
		t.Fatalf("failed to parse include patterns: %v", err)
	}

	excludePatterns, err := ParsePatterns([]string{"*-dev"})
	if err != nil {
		t.Fatalf("failed to parse exclude patterns: %v", err)
	}

	testNamespaces := []string{
		"frontend-prod", "frontend-dev", "backend-prod", "backend-dev",
		"bpfman", "other-service", "kube-system",
	}

	var matched []string
	for _, ns := range testNamespaces {
		included := len(includePatterns) == 0
		for _, pattern := range includePatterns {
			if pattern.match(ns) {
				included = true
				break
			}
		}

		excluded := false
		for _, pattern := range excludePatterns {
			if pattern.match(ns) {
				excluded = true
				break
			}
		}

		if included && !excluded {
			matched = append(matched, ns)
		}
	}

	expected := []string{"frontend-prod", "backend-prod", "bpfman"}
	if len(matched) != len(expected) {
		t.Errorf("expected %d matches, got %d: %v", len(expected), len(matched), matched)
	}

	for _, expectedNs := range expected {
		if !slices.Contains(matched, expectedNs) {
			t.Errorf("expected namespace %s not found in matches: %v", expectedNs, matched)
		}
	}
}
