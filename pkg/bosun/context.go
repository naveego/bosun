package bosun

import (
	"context"
	vault "github.com/hashicorp/vault/api"
	"github.com/naveego/bosun/pkg"
	"github.com/sirupsen/logrus"
	"path/filepath"
	"time"
)

type BosunContext struct {
	Bosun           *Bosun
	Env             *EnvironmentConfig
	Dir             string
	Log             *logrus.Entry
	Values          Values
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
	return c
}

func (c BosunContext) WithAppRepo(a *AppRepo) BosunContext {
	c.AppRepo = a
	return c
}
func (c BosunContext) WithAppRelease(a *AppRelease) BosunContext {
	c.AppRelease = a
	return c
}

func (c BosunContext) WithValues(v Values) BosunContext {
	c.Values = v
	c.valuesAsEnvVars = nil
	return c
}

func (c BosunContext) GetValuesAsEnvVars() map[string]string {
	if c.valuesAsEnvVars == nil && c.Values != nil {
		c.valuesAsEnvVars = c.Values.ToEnv("BOSUN_")
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

func (c BosunContext) ResolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(c.Dir, path)
}

func (c BosunContext) GetParams() Parameters {
	if c.Bosun != nil {
		return c.Bosun.params
	}
	return Parameters{}
}

func (c BosunContext) GetTemplateArgs() pkg.TemplateValues {
	values := c.Values
	values.AddPath("cluster", c.Env.Cluster)
	values.AddPath("domain", c.Env.Domain)

	templateArgs := pkg.TemplateValues{
		Cluster: c.Env.Cluster,
		Domain:  c.Env.Domain,
		Values:  values,
	}
	return templateArgs
}
