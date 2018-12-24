package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"strings"
)

type Release struct {
	Name     string       `yaml:"name"`
	FromPath string       `yaml:"fromPath"`
	Apps     map[string]*AppRelease `yaml:"apps"`
	Fragment *ConfigFragment `yaml:"-"`
}

func (r *Release) SetFragment(f *ConfigFragment) {
	r.FromPath = f.FromPath
	r.Fragment = f
	for _, app := range r.Apps {
		app.Release = r
	}
}

type AppRelease struct {
	Name string `yaml:"name"`
	Repo string `yaml:"repo"`
	RepoPath string `yaml:"repoPath"`
	BosunPath string `yaml:"bosunPath"`
	Branch string `yaml:"branch"`
	Version string `yaml:"version"`
	Tag string `yaml:"tag"`
	Commit string `yaml:"commit"`
	ChartName string `yaml:"chartName"`
	App *App `yaml:"-"`
	Release *Release `yaml:"-"`
}

func (r AppRelease) Validate(ctx BosunContext) []error {

	var errs []error

	out, err := pkg.NewCommand("helm",  "search", r.ChartName, "-v", r.Version).RunOut()
	if err != nil {
		errs = append(errs, errors.Errorf("search for %s@%s failed: %s", r.ChartName, r.Version, err))
	}
	if !strings.Contains(out, r.ChartName) {
		errs = append(errs, errors.Errorf("chart %s@%s not found", r.ChartName, r.Version))
	}

	if r.App.IsThirdParty {
		return errs
	}

	// TODO: validate docker image presence


}



func (a *App) MakeAppRelease() (AppRelease, error) {

	r := AppRelease{
		Name:a.Name,
		BosunPath:a.FromPath,
		Version:a.Version,
		Repo:a.Repo,
		RepoPath:a.RepoPath,
		App:a,
		Branch:a.GetBranch(),
		Commit:a.GetCommit(),
		ChartName:a.getChartName(),
	}

	return r, nil

}