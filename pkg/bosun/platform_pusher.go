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
	BundleDir string
	ManifestDir string
	PushAllApps bool
	PushApp string
	EnvironmentPath string
	Cluster string

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
		for appName, upgrade := range relManifest.UpgradedApps {
			if upgrade {
				apps = append(apps, appName)
			}
		}
	}

	planReq := CreateDeploymentPlanRequest{
		Path: req.BundleDir,
		ManifestDirPath: req.ManifestDir,
		Apps: apps,
	}

	env := p.b.GetCurrentEnvironment()
	ctx := p.b.NewContext()
	for _, cluster := range env.Clusters {
		createPullSecretNamespaces := map[string]bool{}
		for _, ns := range cluster.Namespaces {
			createPullSecretNamespaces[ns.Name] = ns.Name != "kube-system"
		}

		for ns, shouldCreate := range createPullSecretNamespaces {
			if shouldCreate {
				for _, ps := range p.b.GetCurrentEnvironment().PullSecrets {
					err = kube.CreateOrUpdatePullSecret(ctx, cluster.Name, ns, ps)
					if err != nil {
						return fmt.Errorf("could not create pull secret '%s' in cluster '%s' and namespace '%s': %w", ps.Name, cluster.Name, ns, err)
					}
				}
			}
		}
	}

	stopPF := kube.PortForward("vault-dev-0", env.VaultNamespace, 8200)
	defer stopPF()

	creator := NewDeploymentPlanCreator(p.b, p.p)
	plan, err := creator.CreateDeploymentPlan(planReq)
	if err != nil {
		return err
	}

	executor := NewDeploymentPlanExecutor(p.b, p.p)
	err = executor.Execute(ExecuteDeploymentPlanRequest{
		Plan: plan,
		Clusters: map[string]bool{ req.Cluster: true },
	})

	return err
}

