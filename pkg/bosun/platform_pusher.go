package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/sirupsen/logrus"
	"path/filepath"
)

type PlatformPusher struct {
	p   *Platform
	b   *Bosun
	log *logrus.Entry
}

type PlatformPushRequest struct {
	BundleDir       string
	ManifestDir     string
	PushAllApps     bool
	PushApp         string
	EnvironmentPath string
	Cluster         string
}

func NewPlatformPusher(bosun *Bosun, platform *Platform) PlatformPusher {
	return PlatformPusher{
		b: bosun,
		p: platform,
	}
}

func (p *PlatformPusher) Push(req PlatformPushRequest) error {

	relManifestFile := filepath.Join(req.ManifestDir, "manifest.yaml")
	var relManifest ReleaseManifest
	err := yaml.LoadYaml(relManifestFile, &relManifest)
	if err != nil {
		return fmt.Errorf("could not read release manifest file '%s': %w", relManifestFile, err)
	}

	var apps []string
	if req.PushApp != "" {
		apps = []string{req.PushApp}
	} else if req.PushAllApps {
		for appName, _ := range relManifest.AppMetadata {
			apps = append(apps, appName)
		}
	} else {
		pinnedAppManifests, manifestErr := relManifest.GetAppManifestsPinnedToRelease()
		if manifestErr != nil {
			return manifestErr
		}
		for appName := range pinnedAppManifests {
			apps = append(apps, appName)
		}
	}

	planReq := CreateDeploymentPlanRequest{
		Path:            req.BundleDir,
		ManifestDirPath: req.ManifestDir,
		Apps:            apps,
	}

	env := p.b.GetCurrentEnvironment()

	stopPF := kube.PortForward("vault-dev-0", env.VaultNamespace, 8200)
	defer stopPF()

	creator := NewDeploymentPlanCreator(p.b, p.p)
	plan, err := creator.CreateDeploymentPlan(planReq)
	if err != nil {
		return err
	}

	executor := NewDeploymentPlanExecutor(p.b, p.p)
	_, err = executor.Execute(ExecuteDeploymentPlanRequest{
		Plan: plan,
	})

	return err
}
