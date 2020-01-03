package kube

import (
	"github.com/naveego/bosun/pkg"
	"os"
	"runtime"
)

type MinikubeConfig struct {
	HostIP   string `yaml:"hostIP" json:"hostIP"`
	Driver   string `yaml:"driver" json:"driver"`
	DiskSize string `yaml:"diskSize" json:"diskSize"`
	Version  string `yaml:"version" json:"version"`
}

func (c MinikubeConfig) ConfigureKubernetes(ctx CommandContext) error {

	if c.DiskSize == "" {
		c.DiskSize = "40G"
	}
	if c.Version == "" {
		c.Version = "1.14.0"
	}
	if c.Driver == "" {
		c.Driver = "virtualbox"
	}

	err := pkg.NewShellExe("minikube", "ip").RunE()
	if err == nil {
		pkg.Log.Info("Minikube is already running.")
		return nil
	}

	pkg.Log.Info("Resetting virtualbox DHCP leases...")
	_, _ = pkg.NewShellExe("bash", "-c", `kill -9 $(ps aux | grep -i "vboxsvc\|vboxnetdhcp" | awk '{print $2}') 2>/dev/null`).RunOutLog()

	leasePath := os.ExpandEnv("$HOME/.config/VirtualBox/HostInterfaceNetworking-vboxnet0-Dhcpd.leases")
	err = os.RemoveAll(leasePath)
	if err != nil {
		pkg.Log.WithError(err).Warn("Could not delete virtualbox leases, IP address may be incorrect.")
	} else {
		pkg.Log.Info("Deleted virtualbox DHCP leases.")
	}

	ctx.Log.Info("minikube not running, starting minikube...")

	pkg.NewShellExe("minikube config set embed-certs true").MustRun()

	if c.Driver == "none" {
		cmd := pkg.NewShellExe("sudo",
			"minikube",
			"start",
			"--kubernetes-version=v"+c.Version,
			"--vm-driver=none",
			"--extra-config=apiserver.service-node-port-range=80-32000",
		).WithEnvValue("CHANGE_MINIKUBE_NONE_USER", "true")

		err = cmd.RunE()
	} else {
		if runtime.GOOS == "windows" {
			err = pkg.NewShellExe("minikube",
				"start",
				"--memory=16000",
				"--cpus=2",
				"--kubernetes-version=v"+c.Version,
				"--vm-driver", c.Driver,
				"--hyperv-virtual-switch", "Default Switch",
				"--extra-config=apiserver.service-node-port-range=80-32000",
				"--disk-size="+c.DiskSize,
			).RunE()
		} else {
			err = pkg.NewShellExe("minikube",
				"start",
				"--memory=16000",
				"--cpus=2",
				"--kubernetes-version=v"+c.Version,
				"--vm-driver", c.Driver,
				"--extra-config=apiserver.service-node-port-range=80-32000",
				"--disk-size="+c.DiskSize,
				//"-v=7",
			).RunE()
		}
	}

	if err != nil {
		return err
	}
	pkg.NewShellExe("minikube addons enable dashboard").MustRun()
	pkg.NewShellExe("minikube addons enable heapster").MustRun()
	pkg.NewShellExe("minikube addons enable kube-dns").MustRun()

	return nil
}
