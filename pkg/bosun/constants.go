package bosun

import "github.com/pkg/errors"

const (
	EnvEnvironment = "BOSUN_ENVIRONMENT"
	EnvDomain = "BOSUN_DOMAIN"
	EnvCluster = "BOSUN_CLUSTER"
	EnvAppVersion = "BOSUN_APP_VERSION"
)

const (
	ScriptRun = "run"
	ScriptBuild = "build"
	ScriptBuildImage = "buildImage"
	ScriptTest = "test"
)

var ErrNotCloned = errors.New("not cloned")