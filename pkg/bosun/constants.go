package bosun

import "github.com/pkg/errors"

const (
	EnvPrefix      = "BOSUN_"
	EnvEnvironment = "BOSUN_ENVIRONMENT"
	EnvDomain      = "BOSUN_DOMAIN"
	EnvCluster     = "BOSUN_CLUSTER"
	EnvAppVersion  = "BOSUN_APP_VERSION"
	EnvAppCommit   = "BOSUN_APP_COMMIT"
	EnvAppBranch   = "BOSUN_APP_BRANCH"
)

const (
	ScriptRun        = "run"
	ScriptBuild      = "build"
	ScriptBuildImage = "buildImage"
	ScriptTest       = "test"
)

var ErrNotCloned = errors.New("not cloned")
