package cli

type Parameters struct {
	Verbose          bool
	DryRun           bool
	Force            bool
	NoReport         bool
	ForceTests       bool
	ValueOverrides   map[string]string
	FileOverrides    []string
	NoCurrentEnv     bool
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

// EnvironmentVariableGetter can return a map of environment variables.
type EnvironmentVariableGetter interface {
	GetEnvironmentVariables() map[string]string
}
