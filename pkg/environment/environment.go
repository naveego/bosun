package environment

import (
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environmentvariables"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"os"
	"strings"
)

type Environment struct {
	Config

	ClusterName    string
	Cluster        *kube.ClusterConfig
	secretGroups   map[string]*SecretGroup
	secretsContext command.ExecutionContext
}

func (e *Environment) GetValueSetCollection() values.ValueSetCollection {
	if e.ValueOverrides == nil {
		return values.NewValueSetCollection()
	}
	return *e.ValueOverrides
}

// GetAppValueSetCollectionProvider returns a ValuesSetCollectionProvider that will provide any values set collection
// defined in this environment for a specific app. If none is defined, an instance that does nothing will be returned.
func (e *Environment) GetAppValueSetCollectionProvider(appName string) values.ValueSetCollectionProvider {

	if appValueOverride, ok := e.AppValueOverrides[appName]; ok {
		return appValueSetCollectionProvider{
			valueSetCollection:appValueOverride,
		}
	}

	return appValueSetCollectionProvider{
		valueSetCollection:values.NewValueSetCollection(),
	}
}

type appValueSetCollectionProvider struct {
	valueSetCollection values.ValueSetCollection
}

func (a appValueSetCollectionProvider) GetValueSetCollection() values.ValueSetCollection {
	return a.valueSetCollection
}

type Options struct {
	Cluster string
}

func New(config Config, options Options) (*Environment, error) {

	env := &Environment{
		Config:      config,
		ClusterName: options.Cluster,
	}

	return env, nil
}

func (e *Environment) Save() error {
	if e.FromPath == "" {
		// if FromPath was not set this is an old-style environment
		// from a merged config
		return nil
	}

	// save any secret groups which were loaded
	for _, secretGroup := range e.secretGroups {
		if err := secretGroup.Save(e.secretsContext); err != nil {
			return errors.Wrapf(err, "saving loaded secret group %q", secretGroup.Name)
		}
	}

	err := yaml.SaveYaml(e.FromPath, e.Config)
	if err != nil {
		return err
	}

	return nil
}

func (e Environment) Matches(candidate EnvironmentFilterable) bool {
	roles, checkRoles := candidate.GetEnvironmentRoles()
	if checkRoles {
		if !roles.Accepts(e.Role) {
			return false
		}
	}

	name, checkName := candidate.GetEnvironmentName()
	if checkName {
		if name != e.Name {
			return false
		}
	}

	return true
}

func (e *Environment) SwitchToCluster(ctx environmentvariables.EnsureContext, cluster *kube.ClusterConfig) error {

	if cluster == nil {
		return errors.New("cluster parameter was nil")
	}

	if e.Cluster != nil && e.Cluster.Name == cluster.Name {
		return nil
	}

	e.setCurrentCluster(cluster)

	e.ClusterName = cluster.Name
	e.Cluster = cluster

	return e.EnsureCluster(ctx)
}

func (e *Environment) GetClustersForRole(role core.ClusterRole) ([]*kube.ClusterConfig, error) {

	clusters, err := e.Clusters.GetKubeConfigDefinitionsByRole(role)
	return clusters, err
}

func (e *Environment) GetClusterByName(name string) (*kube.ClusterConfig, error) {
	cluster, err := e.Clusters.GetKubeConfigDefinitionByName(name)
	return cluster, err
}

func (e *Environment) setCurrentCluster(cluster *kube.ClusterConfig) {
	e.Cluster = cluster
	e.ClusterName = cluster.Name
}

type EnvironmentFilterable interface {
	GetEnvironmentRoles() (core.EnvironmentRoles, bool)
	GetEnvironmentName() (string, bool)
}

// Ensure resolves and sets all environment variables, and
// sets the cluster, but only if the environment has not already
// been set.
func (e *Environment) Ensure(ctx environmentvariables.EnsureContext) error {

	if os.Getenv(core.EnvEnvironment) == e.Name {
		ctx.Log().Debugf("Environment is already %q, based on value of %s", e.Name, core.EnvEnvironment)

		return e.EnsureCluster(ctx)
	}

	return e.ForceEnsure(ctx)
}

// ForceEnsure resolves and sets all environment variables,
// even if the environment already appears to have been configured.
func (e *Environment) ForceEnsure(ctx environmentvariables.EnsureContext) error {

	ctx = ctx.WithPwd(e.FromPath).(environmentvariables.EnsureContext)

	for _, v := range e.Variables {
		if err := v.Ensure(ctx); err != nil {
			return err
		}
	}

	return e.EnsureCluster(ctx)
}

// Ensure resolves and sets all environment variables, and
// sets the cluster, but only if the environment has not already
// been set.
func (e *Environment) EnsureCluster(ctx environmentvariables.EnsureContext) error {

	if ctx.GetParameters().NoCluster || len(e.Clusters) == 0 {
		return nil
	}

	if e.ClusterName == "" {
		e.ClusterName = os.Getenv(core.EnvCluster)
	}

	if e.ClusterName == "" {
		e.ClusterName = e.DefaultCluster
	}

	var err error
	e.Cluster, err = e.GetClusterByName(e.ClusterName)
	if err != nil {
		return err
	}

	previouslySetCluster := os.Getenv(core.EnvCluster)
	if previouslySetCluster == e.Cluster.Name {
		return nil
	}

	_ = os.Setenv(core.EnvCluster, e.Cluster.Name)

	for _, v := range e.Cluster.Variables {
		if err = v.Ensure(ctx); err != nil {
			return err
		}
	}

	currentContext, _ := pkg.NewShellExe("kubectl config current-context ").RunOut()
	if currentContext != e.Cluster.Name {

		core.SetInternalEnvironmentAndCluster(e.Name, e.Cluster.Name)

		pkg.Log.Infof("Switching to cluster %q", e.Cluster.Name)

		_, err = pkg.NewShellExe("kubectl", "config", "use-context", e.Cluster.Name).RunOut()

		if err != nil && strings.Contains(err.Error(), "no context exists with the name") {

			pkg.Log.Warnf("No context configured for cluster %q, I will try to configure it...", e.Cluster.Name)
			err = e.Clusters.HandleConfigureKubeContextRequest(kube.ConfigureKubeContextRequest{
				Log:              ctx.Log(),
				Name:             e.Cluster.Name,
				Force:            ctx.GetParameters().Force,
				ExecutionContext: ctx,
				PullSecrets:      e.PullSecrets,
			})

			if err != nil {
				return err
			}

			pkg.Log.Warnf("Configured new context for cluster %q, you may need to run this command again.", e.Cluster.Name)
			return nil
		}
		return err
	}

	return nil
}

func (e *Environment) GetVariablesAsMap(ctx environmentvariables.EnsureContext) (map[string]string, error) {

	err := e.Ensure(ctx)
	if err != nil {
		return nil, err
	}

	vars := map[string]string{
		core.EnvEnvironment: e.Name,
	}
	for _, v := range e.Variables {
		vars[v.Name] = v.Value
	}

	if e.Cluster != nil {
		for _, v := range e.Cluster.Variables {
			vars[v.Name] = v.Value
		}
	}

	return vars, nil
}

func (e *Environment) Render(ctx environmentvariables.EnsureContext) (string, error) {

	err := e.Ensure(ctx)
	if err != nil {
		return "", err
	}

	vars, err := e.GetVariablesAsMap(ctx)
	if err != nil {
		return "", err
	}

	s := command.RenderEnvironmentSettingScript(vars)

	return s, nil
}

func (e *Environment) Execute(ctx environmentvariables.EnsureContext) error {

	ctx = ctx.WithPwd(e.FromPath).(environmentvariables.EnsureContext)

	for _, cmd := range e.Commands {
		log := ctx.Log().WithField("name", cmd.Name).WithField("fromPath", e.FromPath)
		if cmd.Exec == nil {
			log.Warn("`exec` not set")
			continue
		}
		log.Debug("Running command...")
		_, err := cmd.Exec.Execute(ctx)
		if err != nil {
			return errors.Errorf("error running command %s: %s", cmd.Name, err)
		}
		log.Debug("ShellExe complete.")

	}

	return nil
}

func (e *Environment) PrepareSecrets(ctx command.ExecutionContext) error {

	e.secretGroups = map[string]*SecretGroup{}
	e.secretsContext = ctx

	return nil
}

func (e *Environment) GetSecretGroupConfig(groupName string) (*SecretGroupConfig, error) {
	for _, group := range e.SecretGroupConfigs {
		if group.Name == groupName {
			return group, nil
		}
	}
	return nil, errors.Errorf("no secret group found with name %q", groupName)
}

func (e *Environment) GetSecretConfig(groupName string, secretName string) (*SecretConfig, error) {
	group, err := e.GetSecretGroupConfig(groupName)
	if err != nil {
		return nil, err
	}
	for _, secret := range group.Secrets {
		if secret.Name == secretName {
			return secret, nil
		}
	}
	return nil, errors.Errorf("group %q had no secret named %q", groupName, secretName)
}

func (e *Environment) getSecretGroup(groupName string) (*SecretGroup, error) {
	if e.secretsContext == nil {
		panic("environment secretContext was nil (did you call PrepareSecrets on the environment?)")
	}

	group, ok := e.secretGroups[groupName]
	if !ok {
		groupConfig, err := e.GetSecretGroupConfig(groupName)
		if err != nil {
			return nil, err
		}
		group, err = groupConfig.LoadSecrets(e.secretsContext)
		if err != nil {
			return nil, err
		}
		e.secretGroups[groupName] = group
	}

	return group, nil
}

func (e *Environment) GetSecretValue(groupName string, secretName string) (string, error) {

	group, err := e.getSecretGroup(groupName)
	if err != nil {
		return "", err
	}

	return group.GetSecretValue(secretName)
}

func (e *Environment) AddOrReKeySecretGroup(groupName, keyCommand string) error {
	group, err := e.GetSecretGroupConfig(groupName)
	if err != nil {
		group = &SecretGroupConfig{
			ConfigShared: core.ConfigShared{
				Name:groupName,
				FromPath:e.FromPath,
			},

		}
		e.SecretGroupConfigs = append(e.SecretGroupConfigs, group)
	}
	group.Key = command.CommandValue{
		Command:command.Command{
			Script:keyCommand,
		},
	}

	return nil
}

func (e *Environment) AddOrUpdateSecret(groupName string, secret *Secret) error {
	group, err := e.getSecretGroup(groupName)
	if err != nil {
		return err
	}

	group.updated = true

	group.values[secret.Name] = secret.Value

	replaced := false
	for i, existing := range group.Secrets{
		if existing.Name == secret.Name {
			group.Secrets[i] = &secret.SecretConfig
			replaced = true
			break
		}
	}
	if !replaced {
		group.Secrets = append(group.Secrets, &secret.SecretConfig)
	}

	return nil
}