package bosun

import (
	"context"
	"fmt"
	vault "github.com/hashicorp/vault/api"
	"github.com/naveego/bosun/pkg"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)


type BosunContext struct {
	Bosun           *Bosun
	Env             *EnvironmentConfig
	Dir             string
	Log             *logrus.Entry
	ReleaseValues   *ReleaseValues
	Release         *Release
	AppRepo         *AppRepo
	AppRelease      *AppRelease
	valuesAsEnvVars map[string]string
	ctx             context.Context
}

func (c BosunContext) WithDir(dirOrFilePath string) BosunContext {
	dir := getDirIfFile(dirOrFilePath)
	c.Dir = dir
	return c
}

func (c BosunContext) Ctx() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func (c BosunContext) WithRelease(r *Release) BosunContext {
	c.Release = r
	c.Log = c.Log.WithField("release", r.Name)
	c.LogLine(1, "[Context] Changed Release.")
	return c
}

func (c BosunContext) WithAppRepo(a *AppRepo) BosunContext {
	if c.AppRepo == a {
		return c
	}
	c.AppRepo = a
	c.Log = c.Log.WithField("repo", a.Name)
	c.Log.Debug("")
	c.LogLine(1, "[Context] Changed AppRepo.")
	return c.WithDir(a.FromPath)
}

func (c BosunContext) WithAppRelease(a *AppRelease) BosunContext {
	if c.AppRelease == a {
		return c
	}
	c.AppRelease = a
	c.Log = c.Log.WithField("app", a.Name)
	c.LogLine(1, "[Context] Changed AppRelease.")
	return c
}

func (c BosunContext) WithReleaseValues(v *ReleaseValues) BosunContext {
	c.ReleaseValues = v
	c.valuesAsEnvVars = nil

	yml, _ := yaml.Marshal(v.Values)
	c.LogLine(1, "[Context] Set release values:\n%s\n", string(yml))


	return c
}

func (c BosunContext) GetValuesAsEnvVars() map[string]string {
	if c.valuesAsEnvVars == nil && c.ReleaseValues != nil {
		c.valuesAsEnvVars = c.ReleaseValues.Values.ToEnv("BOSUN_")
	}
	return c.valuesAsEnvVars
}

func (c BosunContext) WithLog(log *logrus.Entry) BosunContext {
	c.Log = log
	return c
}

func (c BosunContext) GetVaultClient() (*vault.Client, error) {
	return c.Bosun.GetVaultClient()
}

func (c BosunContext) IsDryRun() bool {
	return c.Bosun.params.DryRun
}

func (c BosunContext) WithContext(ctx context.Context) BosunContext {
	c.ctx = ctx
	return c
}

func (c BosunContext) WithTimeout(timeout time.Duration) BosunContext {
	ctx, _ := context.WithTimeout(c.Ctx(), timeout)
	return c.WithContext(ctx)
}

// ResolvePath resolves the path relative to the Dir in this context.
// It will also expand some environment variables:
// $ENVIRONMENT and $BOSUN_ENVIRONMENT => Env.Name
// $DOMAIN AND BOSUN_DOMAIN => Env.Domain
func (c BosunContext) ResolvePath(path string) string {
	path = os.Expand(path, func(name string) string {
		switch name {
		case "ENVIRONMENT", "BOSUN_ENVIRONMENT":
			return c.Env.Name
		case "DOMAIN", "BOSUN_DOMAIN":
			return c.Env.Domain
		default:
			return name
		}
	})

	if !filepath.IsAbs(path) {
		path = filepath.Join(c.Dir, path)
	}
	return path
}

func (c BosunContext) GetParams() Parameters {
	if c.Bosun != nil {
		return c.Bosun.params
	}
	return Parameters{}
}

func (c BosunContext) GetTemplateArgs() pkg.TemplateValues {
	tv := pkg.TemplateValues{
		Cluster: c.Env.Cluster,
		Domain:  c.Env.Domain,
	}
	if c.ReleaseValues != nil {
		values := c.ReleaseValues.Values
		values.SetAtPath("cluster", c.Env.Cluster)
		values.SetAtPath("domain", c.Env.Domain)
		tv.Values = values
	}
	return tv
}

func (c BosunContext) WithEnv(env *EnvironmentConfig) BosunContext {
	c.Env = env
	c.Log = c.Log.WithField("env", env.Name)
	return c
}

func (c BosunContext) LogLine(skip int, format string, args ...interface{}) {
	_, file, line, _ := runtime.Caller(skip)
	c.Log.WithField("loc", fmt.Sprintf("%s:%d", file, line)).Debugf(format, args...)
}

var useMinikubeForDockerOnce = new(sync.Once)

func (c BosunContext) UseMinikubeForDockerIfAvailable() {
	useMinikubeForDockerOnce.Do(func() {
		if err := pkg.NewCommand("minikube", "ip").RunE(); err == nil {

		}
	})
}