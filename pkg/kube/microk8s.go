package kube

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Microk8sConfig struct {
	Channel   string `yaml:"channel"`
}

func (c Microk8sConfig) configureKubernetes(ctx ConfigureKubeContextRequest) error {

	if c.Channel == "" {
		c.Channel =  "1.14/stable"
	}

	_, err := exec.LookPath("microk8s")
	if err != nil {
		return errors.Wrapf(err, "microk8s not found, please install using `sudo snap install microk8s --classic --channel=%s`", c.Channel)
	}

	user := os.Getenv("USER")
	home := os.Getenv("HOME")

	err = pkg.NewShellExe("sudo", "usermod", "-a", "-G", "microk8s", user).RunE()
	if err != nil {
		return err
	}

	err = pkg.NewShellExe("sudo", "chown", "-f", "-R", user, filepath.Join(home, ".kube")).RunE()
	if err != nil {
		return err
	}

	ctx.Log.Info("Modified groups to support microk8s commands, you can use microk8s after your next login or run `su - $USER` and try again.")

	apiserverArgsPath := "/var/snap/microk8s/current/args/kube-apiserver"
	apiserverArgsBytes, err := ioutil.ReadFile(apiserverArgsPath)
	if err != nil {
		return errors.Wrap(err, "trying to check args for apiserver")
	}
	apiserverArgs := string(apiserverArgsBytes)

	if !strings.Contains(apiserverArgs, "service-node-port-range") {
		ctx.Log.Info("Expanding port range available to services")
		err = pkg.NewShellExe("sudo", "bash", "-c", fmt.Sprintf("echo '--service-node-port-range=2080-32767' >> %s", apiserverArgsPath)).RunE()
		if err != nil {
			return errors.Wrap(err, "extending port range for apiserver")
		}

		ctx.Log.Info("Restarting apiserver service to apply port range")
		err = pkg.NewShellExe("sudo systemctl restart snap.microk8s.daemon-apiserver.service").RunE()
		if err != nil {
			return err
		}
	}

	rawConfig, err := pkg.NewShellExe("sudo", "microk8s", "kubectl", "config", "view", "--raw").RunOut()
	if err != nil {
		return errors.Wrap(err, "read microk8s config")
	}

	k8sConfigPath := filepath.Join(home, ".kube/config")
	microk8sConfigPath := filepath.Join(home, ".kube/microk8s.config")
	err = ioutil.WriteFile(microk8sConfigPath, []byte(rawConfig), 0600)
	if err != nil {
		return errors.Wrap(err, "adding microk8s config to default config location")
	}

	rawConfig, err = pkg.NewShellExe("kubectl", "config", "view", "--merge", "--flatten").
		WithEnvValue("KUBECONFIG", fmt.Sprintf("%s:%s", microk8sConfigPath, k8sConfigPath)).
		RunOut()
	if err != nil {
		return errors.Wrap(err, "merging k8s config files")
	}

	err = ioutil.WriteFile(k8sConfigPath, []byte(rawConfig), 0600)
	if err != nil {
		return errors.Wrap(err, "writing merged k8s config to default config location")
	}

	ctx.Log.Info("Configuring addons...")
	err = pkg.NewShellExe("microk8s", "enable", "dns", "storage").RunE()
	if err != nil {
		return errors.Wrap(err, "enabling default addons for microk8s")
	}

	ctx.Log.Infof("deleting old virtualbox network interface")
	err = pkg.NewShellExe("sudo", "ifconfig", "vboxnet0", "down").RunE()
	if err != nil {
		ctx.Log.Debug("No virtualbox network found.")
		// return err
	}

	ctx.Log.Infof("creating loopback IP at 192.168.99.1 for host services")
	err = pkg.NewShellExe("sudo", "ifconfig", "lo:microk8s", "192.168.99.1", "up").RunE()
	if err != nil {
		return err
	}

	ctx.Log.Infof("creating loopback IP at 192.168.99.100 for *.n5o.red/*.naveego.red services")
	err = pkg.NewShellExe("sudo", "ifconfig", "lo:microk8s:red", "192.168.99.100", "up").RunE()
	if err != nil {
		return err
	}

	return nil
}
