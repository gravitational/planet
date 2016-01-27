package util

import "sort"

// StringSliceEquals returns true if a equals b.
// Side-effect: the slice arguments are sorted in-place.
func StringSliceEquals(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Sort(sort.StringSlice(a))
	sort.Sort(sort.StringSlice(b))
	for i, _ := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
