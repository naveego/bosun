package stringsn

func Distinct(strs ...string) []string {
	m := map[string]bool{}
	var out []string
	for _, s := range strs {
		if !m[s] {
			out = append(out, s)
			m[s] = true
		}
	}
	return out
}

func AppendIfNotPresent(strs []string, in string) []string {
	found := false
	for _, s := range strs {
		found = s == in
		if found {
			break
		}
	}
	if found {
		return strs
	}
	return append(strs, in)
}
