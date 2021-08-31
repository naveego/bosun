package bosun

import (
	"github.com/naveego/bosun/pkg/git"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/pkg/errors"
)

type ReleaseCreateSettings struct {
	Version     semver.Version
	Base        *semver.Version
	Description string
}

func (p *Platform) CreateRelease(ctx BosunContext, settings ReleaseCreateSettings) (*ReleaseManifest, error) {
	var err error

	ctx.Log().Info("Creating new release.")

	g, err := git.NewGitWrapper(p.FromPath)
	if err != nil {
		return nil, err
	}

	existing, _ := p.GetReleaseMetadataByVersion(settings.Version)
	if existing != nil {
		return nil, errors.Errorf("release already exists with version %v", settings.Version)
	}

	var manifest *ReleaseManifest

	if settings.Base != nil {
		var base *ReleaseMetadata
		base, err = p.GetReleaseMetadataByVersion(*settings.Base)
		if err != nil {
			return nil, errors.Wrapf(err, "can't base release on %s", settings.Base)
		}

		baseBranch := base.Branch
		err = g.CheckOutBranch(baseBranch)
		if err != nil {
			return nil, errors.Wrapf(err, "can't check out base branch %s for base release %s", baseBranch, base)
		}
	}

	branch := p.MakeReleaseBranchName(settings.Version)
	if err = p.SwitchToReleaseBranch(ctx, branch); err != nil {
		return nil, err
	}

	manifest, err = p.GetCurrentRelease()
	if err != nil {
		ctx.Log().WithError(err).Warnf("Could not get current release, creating new release plan with empty release.")
		manifest = &ReleaseManifest{
			ReleaseMetadata: &ReleaseMetadata{
				Version: settings.Version,
				Branch:  branch,
			},
		}
		manifest.init()
	} else {
		ctx.Log().Infof("Using release %s as base.", manifest)
		manifest.ReleaseMetadata.Version = settings.Version
	}

	manifest.ReleaseMetadata.Branch = branch
	manifest.ReleaseMetadata.Description = settings.Description

	p.SetReleaseManifest(SlotStable, manifest)

	return manifest, nil
}
