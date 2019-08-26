package git

import ()

type BranchName string

func (b BranchName) String() string {
	return string(b)
}

func (b BranchName) GetBranchType(spec BranchSpec) (BranchType, error) {
	return spec.GetBranchType(b)
}
