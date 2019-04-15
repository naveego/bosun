package bosun

import (
	"github.com/hashicorp/go-getter"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type ToolDef struct {
	FromPath string `yaml:"-"`
	Name string `yaml:"name"`
	Description string `yaml:"description"`
	URL string `yaml:"url,omitempty"`
	Cmd map[string]string `yaml:"cmd,omitempty"`
	Installer map[string]Installer `yaml:"installer,omitempty"`
}

type Installer struct {
	Script string `yaml:"script,omitempty"`
	Getter *GetterConfig `yaml:"getter,omitempty"`
}

type GetterConfig struct {
	URL string `yaml:"url"`
	Mappings map[string]string `yaml:"mappings"`
}

func (t ToolDef) GetExecutable() (string, error) {

	var ok bool
	var cmd string
	for key, val := range t.Cmd {
		if strings.Contains(key, runtime.GOOS) {
			cmd = val
			ok = true
		}
	}
	if !ok {
		return "", errors.Errorf("no cmd registered for os %q", runtime.GOOS)
	}

	ex, err := exec.LookPath(cmd)
	return ex, err
}

func (t ToolDef) GetInstaller() (*Installer, error) {

	if t.Installer == nil || len(t.Installer) == 0 {
		return nil, errors.New("no installers registered")
	}

	var ok bool
	var installer Installer
	var oses []string
	for key, val := range t.Installer {
		oses = append(oses, key)
		if strings.Contains(key, runtime.GOOS) {
			installer = val
			ok = true
		}
	}

	if !ok {
		return nil, errors.Errorf("no installer defined for current OS (defined installers: %s)", strings.Join(oses, ", "))
	}

	return &installer, nil
}

func (t ToolDef) RunInstall(ctx BosunContext) error {
	installer, err := t.GetInstaller()
	if err != nil {
		return  errors.Wrap(err, "no installer script available")
	}

	err = installer.Execute(ctx)
	if err != nil {
		return err
	}

	_, err = t.GetExecutable()
	if err != nil {
		return errors.Wrap(err, "install completed, but executable still not found")
	}

	return nil
}

func (i Installer) Execute(ctx BosunContext)  error {
	if i.Script != "" {
		cmd := &Command{Script:i.Script}
		_, err := cmd.Execute(ctx, CommandOpts{StreamOutput:true})
		return err
	}

	if i.Getter != nil {
		tmp, err := ioutil.TempDir(os.TempDir(), "bosun-install")
		if err != nil {
			return err
		}

		ctx.Log.Debugf("Downloading from %s to %s", i.Getter.URL, tmp)

		defer func(){
			ctx.Log.Debugf("Deleting %s", tmp)
			os.RemoveAll(tmp)
		} ()

		err = getter.Get(tmp, i.Getter.URL)
		if err != nil {
			return errors.Errorf("error getting content from %q: %s", i.Getter.URL, err)
		}
		ctx.Log.Debugf("Download complete.")

		for from, to := range i.Getter.Mappings {

			from = filepath.Join(tmp, from)
			to = os.ExpandEnv(to)

			ctx.Log.Debugf("Moving %s to %s.", from, to)
			err = os.Rename(from, to)
			if err != nil {
				return err
			}
		}

		return nil
	}

	return errors.New("no install strategy defined")
}
