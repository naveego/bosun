package cli

type Parameters struct {
	Verbose        bool              `yaml:"verbose"`
	DryRun         bool              `yaml:"dryRun"`
	Force          bool              `yaml:"force"`
	NoReport       bool              `yaml:"noReport"`
	ForceTests     bool              `yaml:"forceTests"`
	ValueOverrides map[string]string `yaml:"valueOverrides"`
	FileOverrides  []string          `yaml:"fileOverrides"`
	// Indicates no environment needs to be loaded in this run.
	NoEnvironment bool `yaml:"noEnvironment"`
	// Indicates no cluster is needed in this run.
	NoCluster        bool     `yaml:"noCluster"`
	ConfirmedEnv     string   `yaml:"confirmedEnv"`
	ProviderPriority []string `yaml:"providerPriority"`
	Sudo             bool     `yaml:"sudo"`
	// Additional parameters not strictly defined.
	Misc map[string]string `yaml:"misc"`
}

type ParametersGetter interface {
	GetParameters() Parameters
}

// Pwder can return the working directory.
type Pwder interface {
	Pwd() string
}

type WithPwder interface {
	Pwder
	WithPwd(pwd string) WithPwder
}

// EnvironmentVariableGetter can return a map of environment variables.
type EnvironmentVariableGetter interface {
	GetEnvironmentVariables() map[string]string
}
