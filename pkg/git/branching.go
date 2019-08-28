package git

import (
	"fmt"
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
	"regexp"
	"strconv"
	"strings"
)

type BranchSpec struct {
	Master  string `yaml:"master"`
	Develop string `yaml:"develop"`
	/*
		  Release is the template for feature branches.
		  The template parameter is:
		  {
			"Version":int,
			"Name":string,
		  }
	*/
	Release string `yaml:"release"`
	/*
		  Feature is the template for feature branches.
		  The template parameter is:
		  {
			"Number":int,
			"Slug":string,
		  }
	*/
	Feature string `yaml:"feature"`
}

type BranchType string
type BranchPart string
type BranchParts map[BranchPart]string

func (b BranchParts) Map() map[string]string {
	out := map[string]string{}
	for k, v := range b {
		out[string(k)] = v
	}
	return out
}

const (
	BranchTypeMaster  = BranchType("master")
	BranchTypeDevelop = BranchType("develop")
	BranchTypeRelease = BranchType("release")
	BranchTypeFeature = BranchType("feature")
	BranchPartSlug    = BranchPart("Slug")
	BranchPartVersion = BranchPart("Version")
	BranchPartNumber  = BranchPart("Number")
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

func (b BranchSpec) RenderRelease(parameters BranchParts) (string, error) {
	template, err := util.RenderTemplate(b.Release, parameters.Map())
	return template, err
}

func (b BranchSpec) GetReleaseNameAndVersion(branch BranchName) (name, version string, err error) {

	var parts BranchParts
	parts, err = decomposeBranch(b.Release, branch)
	name = parts[BranchPartSlug]
	version = parts[BranchPartVersion]
	return
}

func (b BranchSpec) RenderFeature(name, number string) (string, error) {
	template, err := util.RenderTemplate(b.Feature, map[string]string{
		string(BranchPartSlug):   name,
		string(BranchPartNumber): number,
	})
	return template, err
}

func (b BranchSpec) GetIssueNumber(branch BranchName) (int, error) {
	parts, err := decomposeBranch(b.Feature, branch)

	if err != nil {
		return 0, err
	}

	rawNumber, ok := parts[BranchPartNumber]
	if !ok {
		return 0, errors.Errorf("no number in branch %q based on template %q", branch, b.Feature)
	}

	number, err := strconv.Atoi(rawNumber)
	if err != nil {
		return 0, errors.Wrapf(err, "invalid number %q", rawNumber)
	}

	return number, nil
}

func decomposeBranch(template string, branch BranchName) (BranchParts, error) {
	in := map[string]string{
		string(BranchPartSlug):    fmt.Sprintf(`(?P<%s>[^/]+)`, BranchPartSlug),
		string(BranchPartVersion): fmt.Sprintf(`(?P<%s>[^/]+)`, BranchPartVersion),
		string(BranchPartNumber):  fmt.Sprintf(`(?P<%s>[^/]+)`, BranchPartNumber),
	}

	template, err := util.RenderTemplate(template, in)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid template %q", template)
	}

	var re *regexp.Regexp
	re, err = regexp.Compile(template)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid template %q", template)
	}

	match := re.FindStringSubmatch(branch.String())
	if len(match) == 0 {
		return nil, errors.Errorf("no matches found in %q using regexp %q", branch, template)
	}
	result := BranchParts{}
	for i, n := range re.SubexpNames() {
		if i != 0 && n != "" {
			result[BranchPart(n)] = match[i]
		}
	}
	return result, nil
}
