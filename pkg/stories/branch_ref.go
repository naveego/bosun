package stories

import (
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/issues"
)

type BranchRef struct {
	Repo issues.RepoRef
	Branch git.BranchName
}


