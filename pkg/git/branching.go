package git

import (
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
	"regexp"
	"strings"
)

type BranchSpec struct {
	Master  string `yaml:"master"`
	Develop string `yaml:"develop"`
	Release string `yaml:"release"`
	Feature string `yaml:"feature"`
}

type BranchType string

const (
	BranchTypeMaster  = BranchType("master")
	BranchTypeDevelop = BranchType("develop")
	BranchTypeRelease = BranchType("release")
	BranchTypeFeature = BranchType("feature")
)

func (b BranchSpec) IsRelease(branch BranchName) bool {
	t, _ := b.GetBranchType(branch)
	return t == BranchTypeRelease
}
func (b BranchSpec) IsMaster(branch BranchName) bool {
	t, _ := b.GetBranchType(branch)
	return t == BranchTypeMaster
}
func (b BranchSpec) IsDevelop(branch BranchName) bool {
	t, _ := b.GetBranchType(branch)
	return t == BranchTypeDevelop
}
func (b BranchSpec) IsFeature(branch BranchName) bool {
	t, _ := b.GetBranchType(branch)
	return t == BranchTypeFeature
}

func (b BranchSpec) GetBranchType(branch BranchName) (BranchType, error) {
	switch branch.String() {
	case b.Master:
		return BranchTypeMaster, nil
	case b.Develop:
		return BranchTypeDevelop, nil
	default:
		branchPrefix := strings.Split(branch.String(), "/")[0]
		releasePrefix := strings.Split(b.Release, "/")[0]
		featurePrefix := strings.Split(b.Feature, "/")[0]
		switch branchPrefix {
		case releasePrefix:
			return BranchTypeRelease, nil
		case featurePrefix:
			return BranchTypeFeature, nil
		default:
			return "", errors.Errorf("branch %q does not match any branching pattern", branch)
		}
	}
}

func (b BranchSpec) GetRelease(branch BranchName) (name string, version string, err error) {
	template, err := util.RenderTemplate(b.Release, map[string]string{
		"Name":    `(?P<name>[^/]+)`,
		"Version": `(?P<version>[^/]+)`,
	})
	if err != nil {
		return
	}

	var re *regexp.Regexp
	re, err = regexp.Compile(template)
	if err != nil {
		return
	}

	match := re.FindStringSubmatch(branch.String())
	result := make(map[string]string)
	for i, n := range re.SubexpNames() {
		if i != 0 && n != "" {
			result[n] = match[i]
		}
	}
	name = result["name"]
	version = result["version"]
	return
}
