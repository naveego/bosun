package util

// Tabler implementations can be rendered as a table.
type Tabler interface {
	Headers() []string
	Rows() [][]string
}
