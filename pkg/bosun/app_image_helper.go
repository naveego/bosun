package bosun

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/slack"
	"github.com/pkg/errors"
	"strings"
)

func NewAppImageHelper(b *Bosun) AppImageHelper {
	return AppImageHelper{Bosun:b}
}

type AppImageHelper struct {
	Bosun *Bosun
}

type PublishImagesRequest struct {
	App *App
}

func (x AppImageHelper) PublishImages(req PublishImagesRequest) error {

	a := req.App

	ctx := x.Bosun.NewContext()

	var report []string

	tags := []string{"latest", a.Version.String()}

	branch := a.GetBranchName()

	if a.Branching.IsFeature(branch) {
		tag := "unstable-" + featureBranchTagRE.ReplaceAllString(strings.ToLower(branch.String()), "-")
		ctx.Log().WithField("branch", branch).Warnf(`Publishing from feature branch, pushing "unstable" and %q tag only.`, tag)
		tags = []string{"unstable", tag}
	} else if a.Branching.IsDevelop(branch) {
		tags = append(tags, "develop")
	} else if a.Branching.IsMaster(branch) {
		tags = append(tags, "master")
	} else if a.Branching.IsRelease(branch) {
		_, releaseVersion, err := a.Branching.GetReleaseNameAndVersion(branch)
		if err == nil {
			tags = append(tags, fmt.Sprintf("%s-%s", a.Version, releaseVersion))
		}
	} else {
		if ctx.GetParameters().Force {
			ctx.Log().WithField("branch", branch).Warnf(`Non-standard branch format, pushing "unstable" tag only.`)
			tags = []string{"unstable"}
		} else {
			return errors.Errorf("branch %q matches no patterns; use --force flag to publish with 'unstable' tag anyway", branch)
		}
	}

	for _, tag := range tags {
		for _, taggedName := range a.GetTaggedImageNames(tag) {
			ctx.Log().Infof("Tagging and pushing %q", taggedName)
			untaggedName := strings.Split(taggedName, ":")[0]
			_, err := pkg.NewShellExe("docker", "tag", untaggedName, taggedName).Sudo(ctx.GetParameters().Sudo).RunOutLog()
			if err != nil {
				return err
			}
			_, err = pkg.NewShellExe("docker", "push", taggedName).Sudo(ctx.GetParameters().Sudo).RunOutLog()
			if err != nil {
				return err
			}
			report = append(report, fmt.Sprintf("Tagged and pushed %s", taggedName))
		}
	}

	fmt.Println()
	for _, line := range report {
		color.Green("%s\n", line)
	}

	g, _ := a.Repo.LocalRepo.Git()
	changes, _ := g.ExecLines("log", "--pretty=oneline", "-n", "5", "--no-color")

	slack.Notification{
		IconEmoji:":frame_with_picture:",
	}.WithMessage(`Pushed images for %s from branch %s:
%s

Recent commits: 
%s`,
		a.Name,
		branch,
		strings.Join(report, "\n"),
		strings.Join(changes, "\n"),
	).Send()

	return nil
}
