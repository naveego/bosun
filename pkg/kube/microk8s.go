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
	Channel string `yaml:"channel"`
	Remote  bool   `yaml:"remote"`
	SSHKeyLocation string `yaml:"sshKeyLocation,omitempty"`
	SSHDestination string `yaml:"sshDestination,omitempty"`
}

func (c Microk8sConfig) configureKubernetes(ctx ConfigureRequest) error {

	if c.Channel == "" {
		c.Channel = "1.17/stable"
	}

	if c.Remote {
		return c.configureKubernetesRemote(ctx)
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
	apiserverArgs, err := pkg.NewShellExe("sudo", "cat", apiserverArgsPath).RunOut()
	if err != nil {
		return errors.Wrap(err, "trying to check args for apiserver: you probably need to run `su - $USER` to update your creds")
	}

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

	return ConfigureMickok8sNetworking()
}

func ConfigureMickok8sNetworking() error {

	_, err := exec.LookPath("ifconfig")
	if err != nil {
		return errors.Errorf("ifconfig not found; you may need to install it using `sudo apt install net-tools`")
	}

	pkg.Log.Debug("Deleting old virtualbox network interface...")
	err = pkg.NewShellExe("sudo", "ifconfig", "vboxnet0", "down").RunE()
	if err != nil {
		pkg.Log.Debug("No virtualbox network found.")
		// return err
	}

	pkg.Log.Infof("Creating loopback IP at 192.168.99.1 for host services...")
	err = pkg.NewShellExe("sudo", "ifconfig", "lo:microk8s", "192.168.99.1", "up").RunE()
	if err != nil {
		return err
	}

	pkg.Log.Infof("Creating loopback IP at 192.168.99.100 for *.n5o.red/*.naveego.red services...")
	err = pkg.NewShellExe("sudo", "ifconfig", "lo:microk8s:red", "192.168.99.100", "up").RunE()
	if err != nil {
		return err
	}

	pkg.Log.Info("Done.")

	return nil
}

func (c Microk8sConfig) configureKubernetesRemote(ctx ConfigureRequest) error {

	ctx.Log.Info("Microk8s cluster marked with `remote: true`, no local configuration will be done.")

	certString, err := pkg.NewShellExe("lpass", "show", c.SSHKeyLocation, "--field", "Private Key").RunOut()
	if err != nil {
		return errors.Wrapf(err, "getting cert from lpass location %q", c.SSHKeyLocation)
	}

	tempCertDir, err := ioutil.TempDir(os.TempDir(), "cert-*")
	if err != nil {
		return err
	}

	tempCertPath := filepath.Join(tempCertDir, ctx.Brn.ClusterName + ".pem")

	ctx.Log.Infof("temp cert stored at %q, make sure you delete it if this crashes", tempCertPath)
	
// 	defer os.RemoveAll(tempCertDir)

	certBytes := []byte(strings.TrimSpace(certString) + "\n")

	err = ioutil.WriteFile(tempCertPath, certBytes, 0600)
	if err != nil {
		return err
	}
	
	sshPrefix := []string{"ssh", "-i", tempCertPath, c.SSHDestination}

	ctx.Log.Info("Making sure microk8s is installed...")
	installResult, err := pkg.NewShellExeFromSlice(append(sshPrefix, "sudo", "snap", "install", "microk8s", "--channel", c.Channel )...).RunOutLog()

	if err != nil {
		return err
	}

	ctx.Log.Infof("microk8s install result: %s", installResult)

	ctx.Log.Info("Getting kubeconfig from node...")
	kubeconfigResult, err := pkg.NewShellExeFromSlice(append(sshPrefix, "sudo", "microk8s", "config")...).RunOut()

	if err != nil {
		return err
	}

	ctx.Log.Infof("microk8s kubeconfig:\n%s", kubeconfigResult)

	kubeconfig := strings.ReplaceAll( kubeconfigResult,"microk8s", ctx.Brn.ClusterName)

	if  !strings.Contains(ctx.KubeConfigPath, ctx.Brn.ClusterName) {
		return errors.Errorf("kubeconfigPath %q does not contain requested context name %q (this is required to avoid accidentally overwriting some other cluster's config)", ctx.KubeConfigPath, ctx.Brn.ClusterBrn)
	}

	err = ioutil.WriteFile(ctx.KubeConfigPath, []byte(kubeconfig), 0600)
	if err != nil {
		return err
	}


	return nil
}
