package core

import (
	"fmt"
	"github.com/pkg/errors"
	"os"
	"strings"
)

const (
	EnvPrefix          = "BOSUN_"
	EnvEnvironment     = "BOSUN_ENVIRONMENT"
	EnvEnvironmentRole = "BOSUN_ENVIRONMENT_ROLE"
	EnvCluster         = "BOSUN_CLUSTER"
	EnvAppVersion      = "BOSUN_APP_VERSION"
	EnvAppCommit       = "BOSUN_APP_COMMIT"
	EnvAppBranch       = "BOSUN_APP_BRANCH"
	// Variable containing bosun run time environment information,
	// to force child instances of bosun to have the correct environment and cluster
	EnvInternalEnvironmentAndCluster = "BOSUN_INTERNAL_ENVIRONMENT_AND_CLUSTER"
)

func SetInternalEnvironmentAndCluster(environment, cluster string) {
	_ = os.Setenv(EnvInternalEnvironmentAndCluster, fmt.Sprintf("%s/%s", environment, cluster))
}

func GetInternalEnvironmentAndCluster() (environment, cluster string, found bool) {

	if ec, ok := os.LookupEnv(EnvInternalEnvironmentAndCluster); ok {
		// logrus.StandardLogger().Infof("Found internal environment and cluster: %s", ec)
		segs := strings.Split(ec, "/")
		if len(segs) == 2 {
			return segs[0], segs[1], true
		}
	} else {
		// logrus.StandardLogger().Infof("Did not find internal environment and cluster!", ec)
	}
	return "", "", false
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
