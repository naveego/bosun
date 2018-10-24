package pkg

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/fatih/color"
	vault "github.com/hashicorp/vault/api"
	"github.com/manifoldco/promptui"
)

// This file contains models copied from https://github.com/Praqma/helmsman/blob/master/state.go
// They allow a helmsman state file to be deserialized.

// namespace type represents the fields of a namespace
type namespace struct {
	Protected            bool   `yaml:"protected"`
	InstallTiller        bool   `yaml:"installTiller"`
	UseTiller            bool   `yaml:"useTiller"`
	TillerServiceAccount string `yaml:"tillerServiceAccount"`
	CaCert               string `yaml:"caCert"`
	TillerCert           string `yaml:"tillerCert"`
	TillerKey            string `yaml:"tillerKey"`
	ClientCert           string `yaml:"clientCert"`
	ClientKey            string `yaml:"clientKey"`
}

// config type represents the settings fields
type config struct {
	KubeContext    string `yaml:"kubeContext"`
	Username       string `yaml:"username"`
	Password       string `yaml:"password"`
	ClusterURI     string `yaml:"clusterURI"`
	ServiceAccount string `yaml:"serviceAccount"`
	StorageBackend string `yaml:"storageBackend"`
	SlackWebhook   string `yaml:"slackWebhook"`
	ReverseDelete  bool   `yaml:"reverseDelete"`
}

// state type represents the desired state of applications on a k8s cluster.
type state struct {
	Metadata     map[string]string    `yaml:"metadata"`
	Certificates map[string]string    `yaml:"certificates"`
	Settings     config               `yaml:"settings"`
	Namespaces   map[string]namespace `yaml:"namespaces"`
	HelmRepos    map[string]string    `yaml:"helmRepos"`
	Apps         map[string]*release  `yaml:"apps"`
}

// release type representing Helm releases which are described in the desired state
type release struct {
	Name            string   `yaml:"name"`
	Description     string   `yaml:"description"`
	Namespace       string   `yaml:"namespace"`
	Enabled         bool     `yaml:"enabled"`
	Chart           string   `yaml:"chart"`
	Version         string   `yaml:"version"`
	ValuesFile      string   `yaml:"valuesFile"`
	ValuesFiles     []string `yaml:"valuesFiles"`
	SecretFile      string   `yaml:"secretFile"`
	SecretFiles     []string `yaml:"secretFiles"`
	Purge           bool     `yaml:"purge"`
	Test            bool     `yaml:"test"`
	Protected       bool     `yaml:"protected"`
	Wait            bool     `yaml:"wait"`
	Priority        int      `yaml:"priority"`
	TillerNamespace string   `yaml:"tillerNamespace"`
	Set             map[string]string
	SetString       map[string]string `yaml:"setString"`
	NoHooks         bool              `yaml:"noHooks"`
	Timeout         int
}

type HelmsmanCommand struct {
	VaultClient       *vault.Client
	Domain            string
	Cluster           string
	HelmsmanFilePaths []string
	MarketingRelease  string
	Apply             bool
	DryRun            bool
	NoConfirm         bool
	KeepRenderedFile  bool
	NoVault           bool
	Values            map[string]interface{}
	Apps              []string
	Verbose           bool
}

func (r *HelmsmanCommand) Execute() error {

	var (
		err error
	)

	th := TemplateHelper{
		TemplateValues: TemplateValues{
			Domain:  r.Domain,
			Cluster: r.Cluster,
			Values:  r.Values,
		},
	}
	if !r.NoVault {
		th.VaultClient = r.VaultClient
	}

	stateYaml, err := th.LoadMergedYaml(r.HelmsmanFilePaths...)
	if err != nil {
		return err
	}

	stateBytes, err := r.preprocess([]byte(stateYaml))
	if err != nil {
		return err
	}

	if !r.NoConfirm {
		err = r.summarizeAndConfirm(string(stateBytes))
	}

	if r.Apply || r.DryRun {
		err = r.executeHelmsman(string(stateBytes))
	}

	if r.DryRun {
		color.Yellow("This was a dry-run. To apply the release, run the command again with the --apply flag.")
	} else if !r.Apply {
		color.Yellow("This was a summary of the rendered release. To apply the release, run the command again with the --apply flag.")
	}

	return err
}

func (r *HelmsmanCommand) preprocess(stateBytes []byte) ([]byte, error) {
	var (
		err error
	)

	var s state
	err = yaml.Unmarshal(stateBytes, &s)
	if err != nil {
		return nil, err
	}

	if len(r.Apps) > 0 {

		// only include apps on the list

		whitelisted := map[string]*release{}
		for _, name := range r.Apps {
			if app, ok := s.Apps[name]; ok {
				whitelisted[name] = app
			}
		}

		s.Apps = whitelisted
	}

	stateBytes, err = yaml.Marshal(s)
	if err != nil {
		return nil, err
	}

	return stateBytes, nil
}

func (r *HelmsmanCommand) render(tmpl *template.Template) ([]byte, error) {
	var (
		err error
	)

	w := new(strings.Builder)
	err = tmpl.Execute(w, r)
	if err != nil {
		return nil, err
	}

	result := w.String()

	return []byte(result), nil
}

func (r *HelmsmanCommand) summarizeAndConfirm(fileContent string) error {

	color.Blue("Rendered desired state: \n")
	fmt.Println(fileContent)

	if r.Apply {
		prompt := promptui.Prompt{
			Label:     "Proceed with release?",
			IsConfirm: true,
		}

		_, err := prompt.Run()

		if err == promptui.ErrAbort {
			color.Red("User quit.")
			os.Exit(0)
		}
	}

	return nil
}

func (r *HelmsmanCommand) executeHelmsman(fileContent string) error {
	helmsmanTempFileName := fmt.Sprintf("helmsman-temp-%d.yaml", time.Now().Unix())
	helmsmanTempFilePath := filepath.Join(filepath.Dir(r.HelmsmanFilePaths[0]), helmsmanTempFileName)
	err := ioutil.WriteFile(helmsmanTempFilePath, []byte(fileContent), 0600)
	if err != nil {
		return err
	}

	if r.KeepRenderedFile {
		color.Blue("Rendered state file saved to %s", helmsmanTempFilePath)
	} else {
		defer os.Remove(helmsmanTempFilePath)
	}

	args := []string{
		"-f",
		filepath.Base(helmsmanTempFilePath),
		"--no-banner",
		"--keep-untracked-releases",
	}

	if r.Verbose {
		args = append(args, "--verbose", "--debug")
	}

	if r.DryRun {
		args = append(args, "--dry-run")
	} else {
		args = append(args, "--apply")
	}

	cmd := exec.Command("helmsman", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = filepath.Dir(helmsmanTempFilePath)

	err = cmd.Start()
	if err != nil {
		return err
	}

	errs := make(chan error)

	go func() {
		errs <- cmd.Wait()
	}()

	signals := make(chan os.Signal)
	signal.Notify(signals, os.Kill, os.Interrupt)

	select {
	case err = <-errs:
		return err
	case <-signals:
		fmt.Println("User quit.")
		cmd.Process.Kill()
		return nil
	}

	return nil
}
