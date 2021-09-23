package stringsn

func Contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// ContainsOrIsEmpty return true if haystack contains no items or contains needle.
// Otherwise it returns false.
func ContainsOrIsEmpty(haystack []string, needle string) bool {

	if len(haystack) == 0 {
		return true
	}

	return Contains(haystack, needle)
}