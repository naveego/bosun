package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/util/multierr"
	"github.com/naveego/bosun/pkg/vcs"
	yaml2 "github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
)


type mergeTarget struct {
	dir             string
	version         string
	name            string
	fromBranch      string
	toBranch        string
	postMergeAction func(g git.GitWrapper) error
	tags map[string]string
}


func (p *Platform) CommitCurrentRelease(ctx BosunContext) error {

	release, err := p.GetCurrentRelease()
	if err != nil {
		return err
	}

	platformDir, err := git.GetRepoPath(p.FromPath)
	if err != nil {
		return err
	}

	releaseBranch := fmt.Sprintf("release/%s", release.Version)

	progress := map[string]bool{}
	progressFileName := filepath.Join(os.TempDir(), fmt.Sprintf("bosun-release-commit-%s.yaml", release.Version))
	_ = yaml2.LoadYaml(progressFileName, &progress)

	defer func() {
		_ = yaml2.SaveYaml(progressFileName, progress)
	}()

	mergeTargets := map[string]*mergeTarget{
		"devops-develop": {
			dir:        platformDir,
			name:       "devops",
			version:    release.Version.String(),
			fromBranch: releaseBranch,
			toBranch:   "develop",
			tags: map[string]string{
				"":        release.Version.String(),
				"release": release.Name,
			},
		},
		"devops-master": {
			dir:        platformDir,
			name:       "devops",
			version:    release.Version.String(),
			fromBranch: releaseBranch,
			toBranch:   "master",
			tags: map[string]string{
				"":        release.Version.String(),
				"release": release.Name,
			},
		},
	}

	appsNames := map[string]bool{}
	for appName := range release.GetAllAppMetadata() {
		appsNames[appName] = true
	}

	b := ctx.Bosun

	for name := range release.UpgradedApps {
		log := ctx.Log().WithField("app", name)

		appDeploy, appErr := release.GetAppManifest(name)
		if appErr != nil {
			return appErr
		}

		app, appErr := b.GetAppFromProvider(name, WorkspaceProviderName)
		if appErr != nil {
			ctx.Log().WithError(appErr).Errorf("App repo %s (%s) not available.", appDeploy.Name, appDeploy.Repo)
			continue
		}

		if !app.BranchForRelease {
			ctx.Log().Warnf("App repo (%s) for app %s is not branched for release.", app.RepoName, app.Name)
			continue
		}

		// if appDeploy.PinnedReleaseVersion == nil {
		// 	ctx.Log().Warnf("App repo (%s) does not have a release branch pinned, probably not part of this release.", app.RepoName, release.Name, release.Version)
		// 	continue
		// }
		//
		// if *appDeploy.PinnedReleaseVersion != release.Version {
		// 	ctx.Log().Warnf("App repo (%s) is not changed for this release.", app.RepoName)
		// 	continue
		// }

		manifest, appErr := app.GetManifest(ctx)
		if appErr != nil {
			return errors.Wrapf(appErr, "App manifest %s (%s) not available.", appDeploy.Name, appDeploy.Repo)
		}

		if !app.IsRepoCloned() {
			return errors.Errorf("App repo (%s) for app %s is not cloned, cannot merge.", app.RepoName, app.Name)
		}

		appBranch, appErr := app.Branching.RenderRelease(release.GetBranchParts())
		if appErr != nil {
			return appErr
		}

		mt, ok := mergeTargets[app.Repo.Name]
		if !ok {
			masterName := app.Repo.Name
			if progress[masterName] {
				log.Infof("Release version has already been merged to master.")
			} else {
				mt = &mergeTarget{
					dir:        app.Repo.LocalRepo.Path,
					version:    manifest.Version.String(),
					name:       manifest.Name,
					fromBranch: appBranch,
					toBranch:   app.Branching.Master,
					tags:       map[string]string{},
				}
				mt.tags[app.RepoName] = fmt.Sprintf("%s@%s-%s", app.Name, manifest.Version.String(), release.Version.String())
				mergeTargets[masterName] = mt
			}

			if app.Branching.Develop != app.Branching.Master {
				developName := app.RepoName + "-develop"
				if progress[developName] {
					log.Info("Release version has already been merged to develop.")
				} else {

					mergeTargets[developName] = &mergeTarget{
						dir:        app.Repo.LocalRepo.Path,
						version:    manifest.Version.String(),
						name:       manifest.Name,
						fromBranch: appBranch,
						toBranch:   app.Branching.Develop,
						tags:       map[string]string{},
					}
				}
			}
		}
	}

	if len(mergeTargets) == 0 {
		return errors.New("no apps found")
	}

	fmt.Println("About to merge:")
	for label, target := range mergeTargets {
		fmt.Printf("- %s: %s@%s %s -> %s (tags %+v)\n", label, target.name, target.version, target.fromBranch, target.toBranch, target.tags)
	}

	warnings := multierr.New()

	errs := multierr.New()
	// validate that merge will work
	for _, target := range mergeTargets {

		localRepo := &vcs.LocalRepo{Name: target.name, Path: target.dir}

		if localRepo.IsDirty() {
			errs.Collect(errors.Errorf("Repo at %s is dirty, cannot merge.", localRepo.Path))
		}
	}

	if err = errs.ToError(); err != nil {
		return err
	}

	for targetLabel, target := range mergeTargets {

		log := ctx.Log().WithField("repo", target.name)

		localRepo := &vcs.LocalRepo{Name: target.name, Path: target.dir}

		if localRepo.IsDirty() {
			return errors.Errorf("Repo at %s is dirty, cannot merge.", localRepo.Path)
		}

		repoDir := localRepo.Path

		g, _ := git.NewGitWrapper(repoDir)

		err = g.Fetch()
		if err != nil {
			return err
		}

		log.Info("Checking out release branch...")

		_, err = g.Exec("checkout", target.fromBranch)
		if err != nil {
			return errors.Errorf("checkout %s: %s", repoDir, target.fromBranch)
		}

		log.Info("Pulling release branch...")
		err = g.Pull()
		if err != nil {
			return err
		}

		log.Infof("Checking out base branch %s...", target.toBranch)
		_, err = g.Exec("checkout", target.toBranch)
		if err != nil {
			return err
		}

		log.Infof("Pulling base branch %s...", target.toBranch)
		_, err = g.Exec("pull")
		if err != nil {
			return errors.Wrapf(err, "Could not pull branch, you'll need to resolve any merge conflicts.")
		}

		var tags []string
		for _, tag := range target.tags {
			tags = []string{tag}
		}

		var changes string
		changes, err = g.Exec("log", fmt.Sprintf("%s..%s", target.toBranch, target.fromBranch), "--oneline")
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			log.Infof("Branch %q has already been merged into %q.", target.fromBranch, target.toBranch)
		} else {
			tagged := false
			log.Info("Tagging release branch...")
			for _, tag := range tags {
				tagArgs := []string{"tag", tag, "-a", "-m", fmt.Sprintf("Release %s", release.Name)}
				tagArgs = append(tagArgs, "--force")
				_, err = g.Exec(tagArgs...)
				if err != nil {
					log.WithError(err).Warn("Could not tag repo, skipping merge. Set --force flag to force tag.")
				} else {
					tagged = true
				}
			}

			if tagged {
				log.Info("Pushing tags...")

				pushArgs := []string{"push", "--tags"}
				pushArgs = append(pushArgs, "--force")

				_, err = g.Exec(pushArgs...)
				if err != nil {
					return errors.Errorf("push tags: %s", err)
				}
			}

			log.Infof("Merging into branch %s...", target.toBranch)

			_, err = g.Exec("merge", "-m", fmt.Sprintf("Merge %s into %s to commit release %s", target.fromBranch, target.toBranch, release.Version), target.fromBranch)
			for err != nil {

				confirmed := cli.RequestConfirmFromUser("Merge for %s from %s to %s in %s failed, you'll need to complete the merge yourself: %s\nEnter 'y' when you have completed the merge in another terminal, 'n' to abort release commit", targetLabel, target.fromBranch, target.toBranch, repoDir, err)
				if !confirmed {
					_, err = g.Exec("merge", "--abort")
					break
				}

				_, err = g.Exec("merge", "--continue")

			}
		}

		changes, err = g.Exec("log", fmt.Sprintf("origin/%s..%s", target.toBranch, target.fromBranch), "--oneline")
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			log.Infof("Branch %s has already been pushed", target.toBranch)
			progress[targetLabel] = true
			continue
		}

		log.Infof("Pushing branch %s...", target.toBranch)

		_, err = g.Exec("push")
		if err != nil {
			warnings.Collect(errors.Errorf("Push for %s of branch %s failed (you'll need to push it yourself): %s", targetLabel, target.toBranch, err))
			continue
		}

		log.Infof("Merged back to %s and pushed.", target.toBranch)

		progress[targetLabel] = true
	}

	err = warnings.ToError()
	if err != nil {
		return warnings.ToError()
	}

	return nil
}
