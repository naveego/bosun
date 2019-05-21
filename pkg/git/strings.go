package git

import (
	"github.com/pkg/errors"
	"regexp"
)

type BranchName string

var ReleasePattern = regexp.MustCompile("^release/(.*)")
var MasterPattern = regexp.MustCompile("^master$")

func (b BranchName) Release() (string, error) {
	if !b.IsRelease() {
		return "", errors.Errorf("branch %q is not a release branch", b)
	}

	m := ReleasePattern.FindStringSubmatch(string(b))
	return m[1], nil
}

func (b BranchName) IsRelease() bool {
	return ReleasePattern.MatchString(string(b))
}

func (b BranchName) IsMaster() bool {
	return MasterPattern.MatchString(string(b))
}

func (b BranchName) String() string {
	return string(b)
}
