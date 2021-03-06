package core

import (
	"github.com/naveego/bosun/pkg/brns"
	"github.com/pkg/errors"
	"os"
)

const (
	EnvPrefix          = "BOSUN_"
	EnvEnvironment     = "BOSUN_ENVIRONMENT"
	EnvEnvironmentRole = "BOSUN_ENVIRONMENT_ROLE"
	EnvCluster         = "BOSUN_CLUSTER"
	EnvStack           = "BOSUN_STACK"
	EnvAppVersion      = "BOSUN_APP_VERSION"
	EnvAppCommit       = "BOSUN_APP_COMMIT"
	EnvAppBranch       = "BOSUN_APP_BRANCH"
	// Variable containing bosun run time environment information,
	// to force child instances of bosun to have the correct environment and cluster
	EnvInternalStack = "BOSUN_INTERNAL_STACK"
)

func SetInternalBrn(stack brns.StackBrn) {
	_ = os.Setenv(EnvInternalStack, stack.String())
}

func GetInternalEnvironmentAndCluster() (stack brns.StackBrn, found bool) {

	if ec, ok := os.LookupEnv(EnvInternalStack); ok {
		var err error
		stack, err = brns.ParseStack(ec)
		if err == nil {
			return stack, true
		}
	} else {
		// logrus.StandardLogger().Infof("Did not find internal environment and cluster!", ec)
	}
	return brns.StackBrn{}, false
}

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
