package core

import (
	"fmt"
	"github.com/naveego/bosun/pkg/semver"
	"github.com/pkg/errors"
)

var Version string
var Timestamp string
var Commit string

func GetVersion() semver.Version{
	if Version == "" {
		return semver.Version{}
	}

	version, err := semver.NewVersion(Version)
	if err != nil {
		panic(fmt.Sprintf("Invalid version %q. Was this executable built correctly?", Version))
	}
	return version
}

func CheckCompatibility(required semver.Version) error {
	haveVersion := GetVersion()
	if haveVersion.Empty() {
		return nil
	}

	if haveVersion.LessThan(required) {
		return errors.Errorf("version %s is required but bosun is at version %s", required, haveVersion)
	}

	return nil
}