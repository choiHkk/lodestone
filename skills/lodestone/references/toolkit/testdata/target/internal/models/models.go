package models

import "fmt"

func refuseDuplicatePatterns(source string, patterns []string) error {
	seen := make(map[string]bool, len(patterns))
	for _, pattern := range patterns {
		if seen[pattern] {
			return fmt.Errorf("%s: pattern %q is listed twice", source, pattern)
		}
		seen[pattern] = true
	}

	return nil
}
