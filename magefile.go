// +build mage

package main

import (
	"github.com/magefile/mage/sh"
)

// Default target to run when none is specified
// If not set, running mage will list available targets
// var Default = Build

// Publish builds and publishes the docker image docker.n5o.black/public/bosun:latest.
func Publish() error {

	err := sh.RunV("docker", "build", "-t", "docker.n5o.black/public/bosun:latest", ".")
	if err != nil {
		return err
	}

	err = sh.RunV("docker", "push", "docker.n5o.black/public/bosun:latest")

	return err
}

