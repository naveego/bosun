package bosun

import (
	"github.com/naveego/bosun/pkg/actions"
	"github.com/naveego/bosun/pkg/workspace"
)

type Plan []PlanStep

type PlanStep struct {
	Name        string
	Description string
	Action      func(ctx BosunContext) error
}

func (a *AppDeploy) PlanReconciliation(ctx BosunContext) (Plan, error) {

	ctx = ctx.WithAppDeploy(a)

	var steps []PlanStep

	actual, desired := a.ActualState, a.DesiredState

	log := ctx.Log().WithField("name", a.AppManifest.Name)

	log.WithField("state", desired.String()).Debug("Desired state.")
	log.WithField("state", actual.String()).Debug("Actual state.")

	var (
		needsDelete   bool
		needsInstall  bool
		needsRollback bool
		needsUpgrade  bool
	)

	if desired.Status == workspace.StatusNotFound || desired.Status == workspace.StatusDeleted {
		needsDelete = actual.Status != workspace.StatusDeleted && actual.Status != workspace.StatusNotFound
	} else {
		needsDelete = actual.Status == workspace.StatusFailed
		needsDelete = needsDelete || actual.Status == workspace.StatusPendingUpgrade
	}

	if desired.Status == workspace.StatusDeployed {
		if needsDelete {
			needsInstall = true
		} else {
			switch actual.Status {
			case workspace.StatusNotFound:
				needsInstall = true
			case workspace.StatusDeleted:
				needsInstall = true
			case workspace.StatusPendingUpgrade:
				needsInstall = true
			default:
				needsUpgrade = actual.Status != workspace.StatusDeployed
				needsUpgrade = needsUpgrade || actual.Routing != desired.Routing
				needsUpgrade = needsUpgrade || actual.Version != desired.Version
				needsUpgrade = needsUpgrade || actual.Diff != ""
				needsUpgrade = needsUpgrade || desired.Force
			}
		}
	}

	if needsDelete {
		steps = append(steps, PlanStep{
			Name:        "Delete",
			Description: "Delete release from kubernetes.",
			Action:      a.Delete,
		})
	}

	if desired.Status == workspace.StatusDeployed {
		for i := range a.AppManifest.AppConfig.Actions {
			action := a.AppManifest.AppConfig.Actions[i]
			if action.When.Contains(actions.ActionBeforeDeploy) && action.WhereFilter.Matches(ctx.GetMatchMapArgs()) {
				steps = append(steps, PlanStep{
					Name:        action.Name,
					Description: action.Description,
					Action: func(ctx BosunContext) error {
						return action.Execute(ctx)
					},
				})
			}
		}
	}

	if needsInstall {
		steps = append(steps, PlanStep{
			Name:        "Install",
			Description: "Install chart to kubernetes.",
			Action:      a.Install,
		})
	}

	if needsRollback {
		steps = append(steps, PlanStep{
			Name:        "Rollback",
			Description: "Rollback existing release in kubernetes to allow upgrade.",
			Action:      a.Rollback,
		})
	}

	if needsUpgrade {
		steps = append(steps, PlanStep{
			Name:        "Upgrade",
			Description: "Upgrade existing release in kubernetes.",
			Action:      a.Upgrade,
		})
	}

	if desired.Status == workspace.StatusDeployed {
		for i := range a.AppManifest.AppConfig.Actions {
			action := a.AppManifest.AppConfig.Actions[i]
			if action.When.Contains(actions.ActionAfterDeploy) && action.WhereFilter.Matches(ctx.GetMatchMapArgs()) {
				steps = append(steps, PlanStep{
					Name:        action.Name,
					Description: action.Description,
					Action: func(ctx BosunContext) error {
						return action.Execute(ctx)
					},
				})
			}
		}
	}

	return steps, nil

}
