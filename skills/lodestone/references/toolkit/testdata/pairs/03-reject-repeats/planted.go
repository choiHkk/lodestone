package registry

import (
	"fmt"
	"slices"
)

func rejectRepeated(source string, names []string) error {
	sorted := slices.Clone(names)
	slices.Sort(sorted)
	for index := 1; index < len(sorted); index++ {
		if sorted[index] == sorted[index-1] {
			return fmt.Errorf("%s: name %q appears twice", source, sorted[index])
		}
	}

	return nil
}
