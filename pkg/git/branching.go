package git

import (
	"fmt"
	"github.com/naveego/bosun/pkg/templating"
	"github.com/pkg/errors"
	"regexp"
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
			"ID":int,
			"Slug":string,
		  }
	*/
	Feature     string `yaml:"feature"`
	IsDefaulted bool   `yaml:"-"`
}

func (f BranchSpec) MarshalYAML() (interface{}, error) {
	if f.IsDefaulted {
		return nil, nil
	}
	type proxy BranchSpec
	p := proxy(f)

	return &p, nil
}

func (f *BranchSpec) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type proxy BranchSpec
	var p proxy
	if f != nil {
		p = proxy(*f)
	}

	err := unmarshal(&p)

	if err == nil {
		*f = BranchSpec(p)
	}

	return err
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
	BranchPartName    = BranchPart("Name")
)

func (b BranchSpec) WithDefaults() BranchSpec {
	if b.Master == "" {
		b.Master = "master"
	}
	if b.Develop == "" {
		b.Develop = "develop"
	}
	if b.Release == "" {
		b.Release = "release/{{.Version}}"
	}
	if b.Feature == "" {
		b.Feature = "issue/{{.ID}}/{{.Slug}}"
	}
	return b
}

func (b BranchSpec) WithDefaultsFrom(d BranchSpec) BranchSpec {
	if b.Master == "" {
		b.Master = d.Master
	}
	if b.Develop == "" {
		// default behavior is trunk based development
		b.Develop = d.Develop
	}
	if b.Release == "" {
		// migrate BranchForRelease to p.Branching.Release pattern.
		b.Release = d.Release
	}
	if b.Feature == "" {
		b.Feature = b.Feature
	}
	return b
}

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

func (b BranchSpec) GetBranchTemplate(typ BranchType) string {
	switch typ {
	case BranchTypeDevelop:
		return b.Develop
	case BranchTypeRelease:
		return b.Release
	case BranchTypeFeature:
		return b.Feature
	case BranchTypeMaster:
		return b.Master
	default:
		return "unknown"
	}
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
	template, err := templating.RenderTemplate(b.Release, parameters.Map())
	return template, err
}

func (b BranchSpec) GetReleaseNameAndVersion(branch BranchName) (name, version string, err error) {

	var parts BranchParts
	parts, err = decomposeBranch(b.Release, branch)
	name = parts[BranchPartSlug]
	version = parts[BranchPartVersion]
	return
}

func (b BranchSpec) RenderFeature(name string, number interface{}) (string, error) {
	template, err := templating.RenderTemplate(b.Feature, map[string]string{
		string(BranchPartSlug):   name,
		string(BranchPartNumber): fmt.Sprintf("%v", number),
	})
	return template, err
}

func (b BranchSpec) GetIssueNumber(branch BranchName) (string, error) {
	parts, err := decomposeBranch(b.Feature, branch)

	if err != nil {
		return "", err
	}

	rawNumber, ok := parts[BranchPartNumber]
	if !ok {
		return "", errors.Errorf("no number in branch %q based on template %q", branch, b.Feature)
	}

	return rawNumber, nil
}

func decomposeBranch(template string, branch BranchName) (BranchParts, error) {
	in := map[string]string{
		string(BranchPartSlug):    fmt.Sprintf(`(?P<%s>[^/]+)`, BranchPartSlug),
		string(BranchPartVersion): fmt.Sprintf(`(?P<%s>[^/]+)`, BranchPartVersion),
		string(BranchPartNumber):  fmt.Sprintf(`(?P<%s>[^/]+)`, BranchPartNumber),
		string(BranchPartName):    fmt.Sprintf(`(?P<%s>[^/]+)`, BranchPartName),
	}

	template, err := templating.RenderTemplate(template, in)
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
