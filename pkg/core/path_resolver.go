package core

type PathResolver interface {
	ResolvePath(path string, expansions ...string) string
}
