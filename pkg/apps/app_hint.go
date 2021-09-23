package apps

import (
	"fmt"
	"github.com/naveego/bosun/pkg/issues"
)

// AppHint is a hint about a known app, its repo, and possibly where it has been cloned to.
type AppHint struct {
	Name      string         `yaml:"name"`
	Repo      issues.RepoRef `yaml:"repo"`
	LocalPath string        `yaml:"localPath,omitempty"`
}

func (a AppHint) String() string {
	if a.LocalPath == "" {
		return fmt.Sprintf("%s in repo %s", a.Name, a.Repo)
	}
	return fmt.Sprintf("%s in repo %s cloned to %s", a.Name, a.Repo, a.LocalPath)
}

type AppHintList []AppHint


func (a AppHintList) Headers() []string {
	return []string{
		"Name",
		"Repo",
		"Path",
	}
}

func (a AppHintList) Rows() [][]string {

	var out [][]string

	for _, appRef := range a {

		row := []string{
			appRef.Name,
			appRef.Repo.String(),
			appRef.LocalPath,
		}
		out = append(out, row)
	}

	return out
}
