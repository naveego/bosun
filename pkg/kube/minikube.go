package kube

import (
	"fmt"
	"github.com/naveego/bosun/pkg/command"

	"github.com/naveego/bosun/pkg/core"
	"os"
	"runtime"
)

type MinikubeConfig struct {
	HostIP   string `yaml:"hostIP" json:"hostIP"`
	Driver   string `yaml:"driver" json:"driver"`
	DiskSize string `yaml:"diskSize" json:"diskSize"`
	MemoryMB int `yaml:"memoryMB,omitempty"`
	CPUs int `yaml:"cpus,omitempty"`
	Version  string `yaml:"version" json:"version"`
}

func (c MinikubeConfig) configureKubernetes(ctx ConfigureRequest) error {

	if c.DiskSize == "" {
		c.DiskSize = "40G"
	}
	if c.Version == "" {
		c.Version = "1.14.0"
	}
	if c.Driver == "" {
		c.Driver = "virtualbox"
	}
	if c.CPUs == 0 {
		c.CPUs = 2
	}
	if c.MemoryMB == 0 {
		c.MemoryMB = 16000
	}

	fmt.Println(c)

	err := command.NewShellExe("minikube", "ip").RunE()
	if err == nil {
		core.Log.Info("Minikube is already running.")
		return nil
	}

	core.Log.Info("Resetting virtualbox DHCP leases...")
	_, _ = command.NewShellExe("bash", "-c", `kill -9 $(ps aux | grep -i "vboxsvc\|vboxnetdhcp" | awk '{print $2}') 2>/dev/null`).RunOutLog()

	leasePath := os.ExpandEnv("$HOME/.kubeconfig/VirtualBox/HostInterfaceNetworking-vboxnet0-Dhcpd.leases")
	if runtime.GOOS == "darwin" {
		leasePath = os.ExpandEnv("$HOME/Library/VirtualBox/HostInterfaceNetworking-vboxnet0-Dhcpd.leases")
	}
	err = os.RemoveAll(leasePath)
	if err != nil {
		core.Log.WithError(err).Warn("Could not delete virtualbox leases, IP address may be incorrect.")
	} else {
		core.Log.Info("Deleted virtualbox DHCP leases.")
	}

	ctx.Log.Info("minikube not running, starting minikube...")

	// this is disabled because of a bug in minikube:
	// command.NewShellExe("minikube kubeconfig set embed-certs true").MustRun()

	if c.Driver == "none" {
		cmd := command.NewShellExe("sudo",
			"minikube",
			"start",
			"--kubernetes-version=v"+c.Version,
			"--vm-driver=none",
			"--extra-kubeconfig=apiserver.service-node-port-range=80-32000",
		).WithEnvValue("CHANGE_MINIKUBE_NONE_USER", "true")

		err = cmd.RunE()
	} else {
		if runtime.GOOS == "windows" {
			err = command.NewShellExe("minikube",
				"start",
				fmt.Sprintf("--memory=%d", c.MemoryMB),
				fmt.Sprintf("--cpus=%d", c.CPUs),
				"--kubernetes-version=v"+c.Version,
				"--vm-driver", c.Driver,
				"--hyperv-virtual-switch", "Default Switch",
				"--extra-kubeconfig=apiserver.service-node-port-range=80-32000",
				"--disk-size="+c.DiskSize,
			).RunE()
		} else {
			err = command.NewShellExe("minikube",
				"start",
				fmt.Sprintf("--memory=%d", c.MemoryMB),
				fmt.Sprintf("--cpus=%d", c.CPUs),
				"--kubernetes-version=v"+c.Version,
				"--vm-driver", c.Driver,
				"--extra-kubeconfig=apiserver.service-node-port-range=80-32000",
				"--disk-size="+c.DiskSize,
				//"-v=7",
			).RunE()
		}
	}

	if err != nil {
		return err
	}

	return nil
}
