// Package filter provides glob-based path filtering.
package filter

import (
	"github.com/gobwas/glob"
)

// PathFilter filters paths based on include/exclude glob patterns.
type PathFilter struct {
	includes []glob.Glob
	excludes []glob.Glob
}

// NewPathFilter creates a new PathFilter from include and exclude patterns.
func NewPathFilter(includePatterns, excludePatterns []string) (*PathFilter, error) {
	f := &PathFilter{}

	for _, p := range includePatterns {
		g, err := glob.Compile(p, '/')
		if err != nil {
			return nil, err
		}
		f.includes = append(f.includes, g)
	}

	for _, p := range excludePatterns {
		g, err := glob.Compile(p, '/')
		if err != nil {
			return nil, err
		}
		f.excludes = append(f.excludes, g)
	}

	return f, nil
}

// Matches returns true if the path matches the filter criteria.
// A path matches if:
// - It matches at least one include pattern (or no includes are specified)
// - It does not match any exclude pattern
func (f *PathFilter) Matches(path string) bool {
	// Check excludes first
	for _, g := range f.excludes {
		if g.Match(path) {
			return false
		}
	}

	// If no includes specified, include everything
	if len(f.includes) == 0 {
		return true
	}

	// Check includes
	for _, g := range f.includes {
		if g.Match(path) {
			return true
		}
	}

	return false
}

// HasPatterns returns true if any patterns are configured.
func (f *PathFilter) HasPatterns() bool {
	return len(f.includes) > 0 || len(f.excludes) > 0
}
