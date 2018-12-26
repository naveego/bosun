package pkg

import "runtime"

type MinikubeCmd struct {
}

func (m *MinikubeCmd) Up() error {

	err := NewCommand("minikube", "ip").RunE()
	if err == nil {
		Log.Info("minikube already running")
		return nil
	}

	Log.Info("minikube not running, starting minikube...")

	NewCommand("minikube config set embed-certs true").MustRun()

	if runtime.GOOS == "windows" {
		err = NewCommand("minikube",
			"start",
			"--memory=16000",
			"--cpus=2",
			"--kubernetes-version=v1.10.0",
			"--vm-driver=hyperv",
			"--hyperv-virtual-switch", "Default Switch",
			"--extra-config=apiserver.service-node-port-range=80-32000",
		).RunE()
	} else {
		err = NewCommand("minikube",
			"start",
			"--memory=16000",
			"--cpus=2",
			"--kubernetes-version=v1.10.0",
			"--vm-driver=virtualbox",
			"--extra-config=apiserver.service-node-port-range=80-32000",
		).RunE()
	}

	if err != nil {
		return err
	}
	NewCommand("minikube addons enable kube-dns").MustRun()

	return nil
}
