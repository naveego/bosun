package core

import "github.com/pkg/errors"

const (
	EnvPrefix          = "BOSUN_"
	EnvEnvironment     = "BOSUN_ENVIRONMENT"
	EnvEnvironmentRole = "BOSUN_ENVIRONMENT_ROLE"
	EnvCluster         = "BOSUN_CLUSTER"
	EnvAppVersion      = "BOSUN_APP_VERSION"
	EnvAppCommit       = "BOSUN_APP_COMMIT"
	EnvAppBranch       = "BOSUN_APP_BRANCH"
)

const (
	ScriptRun        = "run"
	ScriptBuild      = "build"
	ScriptBuildImage = "buildImage"
	ScriptTest       = "test"
)

var ErrNotCloned = errors.New("not cloned")

const (
	LabelName       = "name"
	LabelPath       = "path"
	LabelBranch     = "branch"
	LabelCommit     = "commit"
	LabelVersion    = "version"
	LabelDeployable = "deployable"
)
