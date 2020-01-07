package cli

type Parameters struct {
	Verbose        bool
	DryRun         bool
	Force          bool
	NoReport       bool
	ForceTests     bool
	ValueOverrides map[string]string
	FileOverrides  []string
	// Indicates no environment needs to be loaded in this run.
	NoEnvironment bool
	// Indicates no cluster is needed in this run.
	NoCluster        bool
	ConfirmedEnv     string
	ProviderPriority []string
	Sudo             bool
	// Additional parameters not strictly defined.
	Misc map[string]string
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
