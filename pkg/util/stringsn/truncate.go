package stringsn

func Truncate(s string, n int) string {
	if len(s) < n {
		return s
	}

	return s[:n]
}
