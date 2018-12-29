package helm

import (
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func PublishChart(chart string, force bool) error {
	stat, err := os.Stat(chart)
	if !stat.IsDir() {
		return errors.Errorf("%q is not a directory", chart)
	}

	chartName := filepath.Base(chart)
	log := pkg.Log.WithField("chart", chart).WithField("@chart", chartName)

	chartText, err := new(pkg.Command).WithExe("helm").WithArgs("inspect", "chart", chart).RunOut()
	if err != nil {
		return errors.Wrap(err, "Could not inspect chart")

	}
	thisVersionMatch := versionExtractor.FindStringSubmatch(chartText)
	if len(thisVersionMatch) != 2 {
		return errors.New("chart did not have version")
	}
	thisVersion := thisVersionMatch[1]

	log = log.WithField("@version", thisVersion)
	qualifiedName := "helm.n5o.black/" + chartName

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

	out, err := pkg.NewCommand("helm", "package", chart).RunOut()
	if err != nil {
		return errors.Wrap(err, "could not create package")
	}

	f := strings.Fields(out)
	packagePath := f[len(f)-1]

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
