// +build mage

package main

import (
	"github.com/magefile/mage/sh"
	"log"
)

// Default target to run when none is specified
// If not set, running mage will list available targets
// var Default = Build

// Publish builds and publishes the docker image docker.n5o.black/public/bosun:latest.
func Publish() error {
	version, err := sh.Output("bosun", "app", "version")
	check(err)

	check(sh.RunV("docker", "build", "-t", "docker.n5o.black/public/bosun:latest", "."))

	check(sh.RunV("docker", "tag", "docker.n5o.black/public/bosun:latest", "docker.n5o.black/public/bosun:"+version))

	check(sh.RunV("docker", "push", "docker.n5o.black/public/bosun:" + version))

	check(sh.RunV("docker", "push", "docker.n5o.black/public/bosun:latest"))

	return nil
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

