// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package tools

import (
	"strings"
	"testing"
)

func TestValidateConcepts(t *testing.T) {
	cases := []struct {
		name      string
		concepts  []string
		prereqs   map[string][]string
		wantErr   string
	}{
		{
			name:     "ok small graph",
			concepts: []string{"a", "b", "c"},
			prereqs:  map[string][]string{"b": {"a"}},
		},
		{
			name:     "empty concept name rejected",
			concepts: []string{"a", "", "c"},
			wantErr:  "empty concept name",
		},
		{
			name:     "concept name too long",
			concepts: []string{strings.Repeat("x", maxConceptNameLen+1)},
			wantErr:  "too long",
		},
		{
			name:     "too many concepts",
			concepts: makeStrings(maxConceptsPerCall + 1),
			wantErr:  "too many concepts",
		},
		{
			name:     "too many prereqs per node",
			concepts: []string{"target"},
			prereqs:  map[string][]string{"target": makeStrings(maxPrereqEntriesPerNode + 1)},
			wantErr:  "too many prerequisites",
		},
		{
			name:     "prereq value too long",
			concepts: []string{"a"},
			prereqs:  map[string][]string{"a": {strings.Repeat("y", maxConceptNameLen+1)}},
			wantErr:  "prerequisite value too long",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConcepts(tc.concepts, tc.prereqs)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestValidateValueFramings(t *testing.T) {
	if err := validateValueFramings(nil); err != nil {
		t.Fatalf("nil framings should be ok, got %v", err)
	}
	ok := &ValueFramingsInput{Financial: "short"}
	if err := validateValueFramings(ok); err != nil {
		t.Fatalf("short framing rejected: %v", err)
	}
	tooLong := &ValueFramingsInput{Financial: strings.Repeat("a", maxValueFramingLen+1)}
	if err := validateValueFramings(tooLong); err == nil {
		t.Fatal("expected error for oversized framing")
	}
}

func makeStrings(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "c"
	}
	return out
}
