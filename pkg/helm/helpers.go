package helm

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ChartHandle string

func (c ChartHandle) String() string {
	return string(c)
}

func (c ChartHandle) HasRepo() bool {
	return strings.Contains(string(c), "/")
}
func (c ChartHandle) WithRepo(repo string) ChartHandle {
	segs := strings.Split(string(c), "/")
	switch len(segs) {
	case 1:
		return ChartHandle(fmt.Sprintf("%s/%s", repo, segs[0]))
	case 2:
		return ChartHandle(fmt.Sprintf("%s/%s", repo, segs[1]))
	}
	panic(fmt.Sprintf("invalid chart %q", string(c)))
}

// PublishChart publishes the chart at path using qualified name.
// If force is true, an existing version of the chart will be overwritten.
func PublishChart(qualifiedName, path string, force bool) error {
	stat, err := os.Stat(path)
	if !stat.IsDir() {
		return errors.Errorf("%q is not a directory", path)
	}

	chartName := filepath.Base(path)
	log := pkg.Log.WithField("chart", path).WithField("@chart", chartName)

	chartText, err := new(pkg.Command).WithExe("helm").WithArgs("inspect", "chart", path).RunOut()
	if err != nil {
		return errors.Wrap(err, "Could not inspect chart")

	}
	thisVersionMatch := versionExtractor.FindStringSubmatch(chartText)
	if len(thisVersionMatch) != 2 {
		return errors.New("chart did not have version")
	}
	thisVersion := thisVersionMatch[1]

	log = log.WithField("@version", thisVersion)

	repoContent, err := new(pkg.Command).WithExe("helm").WithEnvValue("AWS_DEFAULT_PROFILE", "black").WithArgs("search", qualifiedName, "--versions").RunOut()
	if err != nil {
		return errors.Wrap(err, "could not search repo")
	}

	searchLines := strings.Split(repoContent, "\n")
	versionExists := false
	for _, line := range searchLines {
		f := strings.Fields(line)
		if len(f) > 2 && f[1] == thisVersion {
			versionExists = true
		}
	}

	if versionExists && !force {
		return errors.New("version already exists (use --force to overwrite)")
	}

	out, err := pkg.NewCommand("helm", "package", path).RunOut()
	if err != nil {
		return errors.Wrap(err, "could not create package")
	}

	f := strings.Fields(out)
	packagePath := f[len(f)-1]

	defer os.Remove(packagePath)

	helmArgs := []string{"s3", "push", packagePath, "helm.n5o.black"}
	if force {
		helmArgs = append(helmArgs, "--force")
	}

	err = pkg.NewCommand("helm", helmArgs...).WithEnvValue("AWS_DEFAULT_PROFILE", "black").RunE()

	if err != nil {
		return errors.Wrap(err, "could not publish chart")
	}

	return nil
}

var versionExtractor = regexp.MustCompile("version: (.*)")
