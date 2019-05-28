package bosun

import (
	"github.com/naveego/bosun/pkg"
	"runtime"
)

func MinikubeUp(ctx BosunContext) error {

	ws := ctx.Bosun.GetWorkspace()
	cfg := ws.Minikube

	err := pkg.NewCommand("minikube", "ip").RunE()
	if err == nil {
		pkg.Log.Info("minikube already running")
		return nil
	}

	if cfg.Driver == "" {
		cfg.Driver = "virtualbox"
	}

	ctx.Log.Info("minikube not running, starting minikube...")

	pkg.NewCommand("minikube config set embed-certs true").MustRun()

	if cfg.Driver == "none" {
		cmd := pkg.NewCommand("sudo",
			"minikube",
			"start",
			"--kubernetes-version=v1.10.0",
			"--vm-driver=none",
			"--extra-config=apiserver.service-node-port-range=80-32000",
		).WithEnvValue("CHANGE_MINIKUBE_NONE_USER", "true")

		err = cmd.RunE()
	} else {
		if runtime.GOOS == "windows" {
			err = pkg.NewCommand("minikube",
				"start",
				"--memory=16000",
				"--cpus=2",
				"--kubernetes-version=v1.10.0",
				"--vm-driver", cfg.Driver,
				"--hyperv-virtual-switch", "Default Switch",
				"--extra-config=apiserver.service-node-port-range=80-32000",
				"--disk-size="+ws.Minikube.DiskSize,
			).RunE()
		} else {
			err = pkg.NewCommand("minikube",
				"start",
				"--memory=16000",
				"--cpus=2",
				"--kubernetes-version=v1.10.0",
				"--vm-driver", cfg.Driver,
				"--extra-config=apiserver.service-node-port-range=80-32000",
				"--disk-size="+ws.Minikube.DiskSize,
				//"-v=7",
			).RunE()
		}
	}

	if err != nil {
		return err
	}
	pkg.NewCommand("minikube addons enable dashboard").MustRun()
	pkg.NewCommand("minikube addons enable heapster").MustRun()
	pkg.NewCommand("minikube addons enable kube-dns").MustRun()

	return nil
}
