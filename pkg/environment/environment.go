package environment

import (
	"fmt"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environmentvariables"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"os"
	"path/filepath"
	"strings"
)

type Environment struct {
	Config

	ClusterName  string
	cluster      *kube.Cluster
	stack        *kube.Stack
	secretGroups map[string]*SecretGroup
}

func (e *Environment) Cluster() *kube.Cluster {
	if e.cluster == nil {
		panic("environment was built without a cluster")
	}
	return e.cluster
}

func (e *Environment) Stack() *kube.Stack {
	if e.stack == nil {
		panic("environment was built without a stack")
	}
	return e.stack
}

func (e *Environment) GetClusterConfig() *kube.ClusterConfig {
	return &e.Cluster().ClusterConfig
}

func (e *Environment) GetValueSetCollection() values.ValueSetCollection {
	if e.ValueOverrides == nil {
		return values.NewValueSetCollection()
	}
	return *e.ValueOverrides
}

// IsAppDisabled returns true if the app is disabled for the environment.
// DeployedApps are assumed to be disabled for the environment unless they are in the app list and not marked as disabled
func (e Environment) IsAppDisabled(appName string) bool {
	v, ok := e.Apps[appName]
	return !ok || v.Disabled
}

// GetAppValueSetCollectionProvider returns a ValuesSetCollectionProvider that will provide any values set collection
// defined in this environment for a specific app. If none is defined, an instance that does nothing will be returned.
func (e *Environment) GetAppValueSetCollectionProvider(appName string) values.ValueSetCollectionProvider {

	if appValueOverride, ok := e.Apps[appName]; ok {
		return appValueSetCollectionProvider{
			valueSetCollection: appValueOverride,
		}
	}

	return appValueSetCollectionProvider{
		valueSetCollection: values.NewValueSetCollection(),
	}
}

type appValueSetCollectionProvider struct {
	valueSetCollection values.ValueSetCollection
}

func (a appValueSetCollectionProvider) GetValueSetCollection() values.ValueSetCollection {
	return a.valueSetCollection
}

func (e *Environment) Save() error {
	if e.FromPath == "" {
		// if FromPath was not set this is an old-style environment
		// from a merged config
		return nil
	}

	// save any secret groups which were loaded
	for _, secretGroup := range e.secretGroups {
		if err := secretGroup.Save(); err != nil {
			return err
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

func (e *Environment) ClusterForRoleExists(role core.ClusterRole) bool {
	for _, c := range e.Clusters {
		if c.Roles.Contains(role) {
			return true
		}
	}

	return false
}

type EnvironmentFilterable interface {
	GetEnvironmentRoles() (core.EnvironmentRoles, bool)
	GetEnvironmentName() (string, bool)
}

func (e *Environment) GetVariablesAsMap(ctx environmentvariables.Dependencies) (map[string]string, error) {

	vars := map[string]string{
		core.EnvEnvironment: e.Name,
	}
	for _, v := range e.Variables {
		vars[v.Name] = v.Value
	}

	if e.stack != nil {
		for _, v := range e.Stack().StackTemplate.Variables {
			vars[v.Name] = v.Value
		}
	}

	return vars, nil
}

func (e *Environment) Render(ctx environmentvariables.Dependencies) (string, error) {

	vars, err := e.GetVariablesAsMap(ctx)
	if err != nil {
		return "", err
	}

	aliases := map[string]string{}

	if e.stack != nil {

		for role, namespace := range e.Stack().StackTemplate.Namespaces {
			vars["BOSUN_NAMESPACE_"+strings.ToUpper(string(role))] = namespace.Name
		}

		vars["BOSUN_STACK"] = e.Stack().Name
	}

	if e.stack != nil {

		vars["KUBECONFIG"] = os.ExpandEnv(e.Cluster().GetKubeconfigPath())
		vars["BOSUN_CLUSTER_NAME"] = e.Cluster().Name
		vars["BOSUN_CLUSTER_BRN"] = e.Cluster().Brn.String()
		vars["BOSUN_STACK_BRN"] = e.Stack().Brn.String()
	}

	s := command.RenderEnvironmentSettingScript(vars, aliases)

	return s, nil
}

func (e *Environment) Execute(ctx environmentvariables.Dependencies) error {

	ctx = ctx.WithPwd(e.FromPath).(environmentvariables.Dependencies)

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

func (e *Environment) GetSecretGroupConfig(groupName string) (*SecretGroupConfig, error) {

	secretGroupFilePath, ok := e.SecretGroupFilePaths[groupName]
	if !ok {
		return nil, errors.Errorf("no secret group found with name %q", groupName)
	}

	secretGroupFilePath = filepath.Join(filepath.Dir(e.FromPath), secretGroupFilePath)

	var secretGroupConfig SecretGroupConfig
	if err := yaml.LoadYaml(secretGroupFilePath, &secretGroupConfig); err != nil {
		return nil, err
	}

	secretGroupConfig.SetFromPath(secretGroupFilePath)

	return &secretGroupConfig, nil
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
	group, ok := e.secretGroups[groupName]
	if !ok {
		groupConfig, err := e.GetSecretGroupConfig(groupName)
		if err != nil {
			return nil, err
		}
		group, err = NewSecretGroup(groupConfig)
		if err != nil {
			return nil, err
		}

		if e.secretGroups == nil {
			e.secretGroups = map[string]*SecretGroup{}
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

	return group.GetSecretValue(secretName, nil)
}

// AddSecretGroup creates or replaces a secret group using the provided  key config.
func (e *Environment) AddSecretGroup(groupName string, keyConfig *SecretKeyConfig) error {
	groupConfig, err := e.GetSecretGroupConfig(groupName)
	if err != nil {
		if e.SecretGroupFilePaths == nil {
			e.SecretGroupFilePaths = map[string]string{}
		}
		groupFilepath := fmt.Sprintf("%s.secrets.yaml", groupName)

		groupConfig = &SecretGroupConfig{
			ConfigShared: core.ConfigShared{
				Name:     groupName,
				FromPath: e.ResolveRelative(groupFilepath),
			},
			isNew: true,
			Key:   keyConfig,
		}
		e.SecretGroupFilePaths[groupName] = groupFilepath
		err = e.Save()
		if err != nil {
			return err
		}
	}

	group, err := NewSecretGroup(groupConfig)
	if err != nil {
		return err
	}
	group.values.Dirty = true

	err = group.Save()
	if err != nil {
		return err
	}

	err = e.Save()
	return err
}

func (e *Environment) DeleteSecretGroup(groupName string) error {

	if groupFilePath, ok := e.SecretGroupFilePaths[groupName]; ok {
		groupFilePath = e.ResolveRelative(groupFilePath)
		_ = os.Remove(groupFilePath)
	}

	delete(e.SecretGroupFilePaths, groupName)

	return e.Save()

}

// GetSecretGroup gets the secret group with the provided name. If the group does not exist,
// the returned bool will be false. If the group could not be loaded, the error will not be nil.
func (e *Environment) GetSecretGroup(name string) (group *SecretGroup, exists bool, loadErr error) {
	groupConfig, err := e.GetSecretGroupConfig(name)
	if err != nil {
		return nil, false, nil
	}

	group, loadErr = NewSecretGroup(groupConfig)

	return group, true, loadErr
}

func (e *Environment) AddOrUpdateSecretValue(groupName string, secretName string, value string) error {
	group, err := e.getSecretGroup(groupName)
	if err != nil {
		return err
	}

	return group.AddOrUpdateSecretValue(secretName, value)
}

func (e *Environment) ResolveSecretPath(secretPath string) (string, error) {

	sp, err := ParseSecretPath(secretPath)
	if err != nil {
		return "", err
	}

	group, err := e.getSecretGroup(sp.GroupName)
	if err != nil {
		return "", err
	}

	return group.GetSecretValue(sp.SecretName, sp.Generation)

}

func (e *Environment) ValidateSecrets(secretPaths ...string) error {
	for _, secretPath := range secretPaths {

		_, err := e.ResolveSecretPath(secretPath)
		if err != nil {
			return err
		}
	}

	return nil
}

func (e *Environment) GetSecretGroupConfigs() ([]SecretGroupConfig, error) {
	var out []SecretGroupConfig
	for name := range e.SecretGroupFilePaths {
		group, err := e.GetSecretGroupConfig(name)
		if err != nil {
			return nil, err
		}
		out = append(out, *group)
	}
	return out, nil
}

func (e *Environment) ValidateConsistency() error {

	ambientEnv := os.Getenv(core.EnvEnvironment)
	ambientCluster := os.Getenv(core.EnvCluster)
	ambientStack := os.Getenv(core.EnvStack)
	ambientKubeConfig := os.Getenv("KUBECONFIG")

	var inconsistencies []string

	if e.Name != ambientEnv {
		inconsistencies = append(inconsistencies, fmt.Sprintf("Configured environment is %q but %s=%s", e.Name, core.EnvEnvironment, ambientEnv))
	}

	actualCluster := ""
	actualStack := kube.DefaultStackName
	actualKubeConfig := ambientKubeConfig
	if e.cluster != nil {
		actualCluster = e.cluster.Name
		actualKubeConfig = e.cluster.GetKubeconfigPath()
	}

	if e.stack != nil {
		actualStack = e.stack.Name
	}

	if actualCluster != ambientCluster {
		inconsistencies = append(inconsistencies, fmt.Sprintf("Configured cluster is %q but %s=%s", actualCluster, core.EnvCluster, ambientCluster))
	}

	if actualStack != ambientStack {
		inconsistencies = append(inconsistencies, fmt.Sprintf("Configured stack is %q but %s=%s", actualStack, core.EnvStack, ambientStack))
	}

	if actualKubeConfig != ambientKubeConfig {
		inconsistencies = append(inconsistencies, fmt.Sprintf("Configured kubeconfig path is %q but KUBECONFIG=%s", actualKubeConfig, ambientKubeConfig))
	}

	if len(inconsistencies) > 0 {

		warning, _ := pterm.DefaultBigText.WithLetters(pterm.NewLettersFromStringWithStyle("Warning", pterm.NewStyle(pterm.FgRed))).Srender()

		_, _ = fmt.Fprintf(os.Stderr, warning)

		warningStyle := pterm.NewStyle(pterm.FgYellow)
		_, _ = fmt.Fprintln(os.Stderr, warningStyle.Sprintf("\nInconsistencies detected between bosun config and current env vars. You may want to switch environments using bosun env use ..."))
		for _, m := range inconsistencies {
			_, _ = fmt.Fprintln(os.Stderr, warningStyle.Sprintf("- %s", m))
		}

		confirmed := cli.RequestConfirmFromUser("Do you want to continue despite environment mismatch?")
		if !confirmed {
			return errors.New("user canceled")
		}
	}
	return nil
}
