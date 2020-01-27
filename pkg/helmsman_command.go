package pkg

import (
	"fmt"
	"github.com/naveego/bosun/pkg/templating"
	"github.com/naveego/bosun/pkg/yaml"
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
	Protected            bool   `yaml:"protected" json:"protected"`
	InstallTiller        bool   `yaml:"installTiller" json:"installTiller"`
	UseTiller            bool   `yaml:"useTiller" json:"useTiller"`
	TillerServiceAccount string `yaml:"tillerServiceAccount" json:"tillerServiceAccount"`
	CaCert               string `yaml:"caCert" json:"caCert"`
	TillerCert           string `yaml:"tillerCert" json:"tillerCert"`
	TillerKey            string `yaml:"tillerKey" json:"tillerKey"`
	ClientCert           string `yaml:"clientCert" json:"clientCert"`
	ClientKey            string `yaml:"clientKey" json:"clientKey"`
}

// config type represents the settings fields
type config struct {
	KubeContext    string `yaml:"kubeContext" json:"kubeContext"`
	Username       string `yaml:"username" json:"username"`
	Password       string `yaml:"password" json:"password"`
	ClusterURI     string `yaml:"clusterURI" json:"clusterURI"`
	ServiceAccount string `yaml:"serviceAccount" json:"serviceAccount"`
	StorageBackend string `yaml:"storageBackend" json:"storageBackend"`
	SlackWebhook   string `yaml:"slackWebhook" json:"slackWebhook"`
	ReverseDelete  bool   `yaml:"reverseDelete" json:"reverseDelete"`
}

// state type represents the desired state of applications on a k8s cluster.
type state struct {
	Metadata     map[string]string    `yaml:"metadata" json:"metadata"`
	Certificates map[string]string    `yaml:"certificates" json:"certificates"`
	Settings     config               `yaml:"settings" json:"settings"`
	Namespaces   map[string]namespace `yaml:"namespaces" json:"namespaces"`
	HelmRepos    map[string]string    `yaml:"helmRepos" json:"helmRepos"`
	Apps         map[string]*release  `yaml:"apps" json:"apps"`
}

// release type representing Helm releases which are described in the desired state
type release struct {
	Name            string   `yaml:"name" json:"name"`
	Description     string   `yaml:"description" json:"description"`
	Namespace       string   `yaml:"namespace" json:"namespace"`
	Enabled         bool     `yaml:"enabled" json:"enabled"`
	Chart           string   `yaml:"chart" json:"chart"`
	Version         string   `yaml:"version" json:"version"`
	ValuesFile      string   `yaml:"valuesFile" json:"valuesFile"`
	ValuesFiles     []string `yaml:"valuesFiles" json:"valuesFiles"`
	SecretFile      string   `yaml:"secretsFile" json:"secretsFile"`
	SecretFiles     []string `yaml:"secretsFiles" json:"secretsFiles"`
	Purge           bool     `yaml:"purge" json:"purge"`
	Test            bool     `yaml:"test" json:"test"`
	Protected       bool     `yaml:"protected" json:"protected"`
	Wait            bool     `yaml:"wait" json:"wait"`
	Priority        int      `yaml:"priority" json:"priority"`
	TillerNamespace string   `yaml:"tillerNamespace" json:"tillerNamespace"`
	Set             map[string]string
	SetString       map[string]string `yaml:"setString" json:"setString"`
	NoHooks         bool              `yaml:"noHooks" json:"noHooks"`
	Timeout         int
}

type HelmsmanCommand struct {
	VaultClient       *vault.Client
	Domain            string
	Cluster           string
	HelmsmanFilePaths []string
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
		TemplateValues: templating.TemplateValues{
			Domain:  r.Domain,
			Cluster: r.Cluster,
			Values:  r.Values,
		},
	}

	th.TemplateValues.Values["domain"] = r.Domain
	th.TemplateValues.Values["cluster"] = r.Cluster

	if !r.NoVault {
		th.VaultClient = r.VaultClient
	}

	Log.WithField("paths", r.HelmsmanFilePaths).Debug("Loading helmsman files from paths.")

	stateYaml, err := th.LoadMergedYaml(r.HelmsmanFilePaths...)
	if err != nil {
		return err
	}

	Log.Debug("Loaded DSF from paths.")

	stateBytes, err := r.preprocess([]byte(stateYaml))
	if err != nil {
		return err
	}

	Log.Debug("Processed DSF templates...")

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
