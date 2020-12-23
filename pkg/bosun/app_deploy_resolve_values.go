package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
)

// GetResolvedValues handles loading and merging all values needed for the
// deployment of the app, including reading the default helm chart values,
// loading any values files, and resolving any dynamic values.
func (a *AppDeploy) GetResolvedValues(ctx BosunContext) (*values.PersistableValues, error) {

	matchArgs := ctx.GetMatchMapArgs()
	bosunValues := values.Values{}
	for k, v := range matchArgs {
		bosunValues[k] = v
	}

	resolvedValues := values.NewValueSet().WithValues(
		values.ValueSet{
			Source: "bosun context",
			Static: bosunValues,
		})

	// Make environment values available
	if err := resolvedValues.Static.AddEnvAsPath(core.EnvPrefix, core.EnvAppVersion, a.AppManifest.Version); err != nil {
		return nil, err
	}
	if err := resolvedValues.Static.AddEnvAsPath(core.EnvPrefix, core.EnvAppBranch, a.AppManifest.Branch); err != nil {
		return nil, err
	}
	if err := resolvedValues.Static.AddEnvAsPath(core.EnvPrefix, core.EnvAppCommit, a.AppManifest.Hashes.Commit); err != nil {
		return nil, err
	}

	if chartValues, err := a.AppManifest.AppConfig.LoadChartValues(); err != nil {
		return nil, errors.Wrapf(err, "load chart values")
	} else {
		resolvedValues = resolvedValues.WithValues(chartValues.WithSource("chart values file"))
	}

	if appConfigValues, err := ResolveValues(a.AppConfig, ctx); err != nil {
		return nil, errors.Wrapf(err, "load value set from app config")
	} else {
		resolvedValues = resolvedValues.WithValues(appConfigValues.WithDefaultSource("bosun file"))
	}

	for _, v := range a.AppDeploySettings.ValueSets {
		resolvedValues = resolvedValues.WithValues(v.WithDefaultSource("app deploy settings"))
	}

	if platformValues, err := ResolveValues(ctx.GetPlatform(), ctx); err != nil {
		return nil, errors.Wrapf(err, "resolve platform values")
	} else {
		resolvedValues = resolvedValues.WithValues(platformValues.WithSource("platform overrides"))
	}

	env := ctx.Environment()
	if environmentValues, err := ResolveValues(env, ctx); err != nil {
		return nil, errors.Wrapf(err, "resolve environment values")
	} else {
		resolvedValues = resolvedValues.WithValues(environmentValues.WithDefaultSource(fmt.Sprintf("%s environment", env.Name)))
	}

	if environmentAppValues, err := ResolveValues(env.GetAppValueSetCollectionProvider(a.Name), ctx); err != nil {
		return nil, errors.Wrapf(err, "resolve environment app values")
	} else {
		resolvedValues = resolvedValues.WithValues(environmentAppValues.WithDefaultSource(fmt.Sprintf("%s environment app value overrides", env.Name)))
	}

	cluster := env.Cluster
	if clusterValues, err := ResolveValues(cluster, ctx); err != nil {
		return nil, errors.Wrapf(err, "resolve cluster values")
	} else {
		resolvedValues = resolvedValues.WithValues(clusterValues.WithDefaultSource(fmt.Sprintf("%s cluster", cluster.Name)))
	}

	if clusterAppValues, err := ResolveValues(cluster.GetAppValueSetCollectionProvider(a.Name), ctx); err != nil {
		return nil, errors.Wrapf(err, "resolve cluster app values")
	} else {
		resolvedValues = resolvedValues.WithValues(clusterAppValues.WithDefaultSource(fmt.Sprintf("%s cluster app overrides", cluster.Name)))
	}

	// ApplyToValues any overrides from parameters passed to this invocation of bosun.
	for k, v := range ctx.GetParameters().ValueOverrides {
		var err error
		resolvedValues, err = resolvedValues.WithValueSetAtPath(k, v, "command line parameter")
		if err != nil {
			return nil, errors.Errorf("applying overrides with path %q: %s", k, err)
		}
	}

	// resolve dynamic values
	resolvedValues, err := resolvedValues.WithDynamicValuesResolved(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "resolve dynamic values")
	}

	// Finally apply any value mappings
	err = a.AppManifest.AppConfig.ValueMappings.ApplyToValues(resolvedValues.Static)
	if err != nil {
		return nil, err
	}
	r := &values.PersistableValues{
		Attribution: resolvedValues.StaticAttributions,
		Values: resolvedValues.Static,
	}

	// resolvedDump, _ := yaml.MarshalString(resolvedValues)
	//
	// fmt.Println("Resolved values:")
	// fmt.Println(resolvedDump)
	// fmt.Println()

	return r, nil
}