package stringsn

func Contains(target string, options []string) bool {
	for _, s := range options {
		if s == target {
			return true
		}
	}
	return false
}
