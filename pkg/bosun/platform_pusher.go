package bosun

import (
	"github.com/sirupsen/logrus"
)

type PlatformPusher struct {
	p   *Platform
	b   *Bosun
	log *logrus.Entry
}

type PlatformPushRequest struct {
	BundleDir string
	EnvironmentPath string
}

func NewPlatformPusher(bosun *Bosun, platform *Platform) PlatformPusher {
	return PlatformPusher{
		b: bosun,
		p: platform,
	}
}

func (p *PlatformPusher) Push(req PlatformPushRequest) error {

	planReq := CreateDeploymentPlanRequest{
		Path: req.BundleDir,
		Apps: []string{"dq-ui"},
	}

	creator := NewDeploymentPlanCreator(p.b, p.p)
	plan, err := creator.CreateDeploymentPlan(planReq)
	if err != nil {
		return err
	}

	executor := NewDeploymentPlanExecutor(p.b, p.p)
	executor.Execute(ExecuteDeploymentPlanRequest{
		Plan: plan,
		PreviewOnly: true,
	})

	return err
}

