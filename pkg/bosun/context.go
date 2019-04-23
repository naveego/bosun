package bosun

import (
	"context"
	"fmt"
	vault "github.com/hashicorp/vault/api"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
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
	contextValues   map[string]interface{}
}

// NewTestBosunContext creates a new BosunContext for testing purposes.
// In production code a context should always be obtained from a Bosun instance.
func NewTestBosunContext() BosunContext {
	dir, _ := os.Getwd()
	testBosun := &Bosun{
		vaultClient: &vault.Client{},
	}
	return BosunContext{
		ctx:   context.Background(),
		Dir:   dir,
		Env:   &EnvironmentConfig{},
		Bosun: testBosun,
		Log:   logrus.WithField("TEST", "INSTANCE"),
	}
}

func (c BosunContext) WithDir(dirOrFilePath string) BosunContext {
	if dirOrFilePath == "" {
		panic("empty path passed to WithDir (probably some config didn't get its FromPath property populated)")
	}
	dir := getDirIfFile(dirOrFilePath)
	if c.Dir != dir {
		c.LogLine(1, "[Context] Set dir to %q", dir)
		c.Dir = dir
	}
	return c
}

func (c BosunContext) Ctx() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func (c BosunContext) WithKeyedValue(key string, value interface{}) BosunContext {
	c.LogLine(1, "[Context] Set value at %q to %T", key, value)
	kvs := map[string]interface{}{}
	for k, v := range c.contextValues {
		kvs[k] = v
	}
	kvs[key] = value
	c.contextValues = kvs
	return c
}

func (c BosunContext) GetKeyedValue(key string) interface{} {
	return c.contextValues[key]
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
	if c.valuesAsEnvVars == nil {
		if c.ReleaseValues != nil {
			c.valuesAsEnvVars = c.ReleaseValues.Values.ToEnv("BOSUN_")
		} else {
			c.valuesAsEnvVars = map[string]string{
				EnvCluster: c.Env.Cluster,
				EnvDomain:  c.Env.Domain,
			}
		}
	}
	return c.valuesAsEnvVars
}

func (c BosunContext) WithLog(log *logrus.Entry) BosunContext {
	c.Log = log
	return c
}

func (c BosunContext) WithLogField(key string, value interface{}) BosunContext {
	c.Log = c.Log.WithField(key, value)
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
		values.MustSetAtPath("cluster", c.Env.Cluster)
		values.MustSetAtPath("domain", c.Env.Domain)
		tv.Values = values
	} else {
		tv.Values = Values{}
	}

	return tv
}

func (c BosunContext) GetTemplateHelper() (*pkg.TemplateHelper, error) {
	vaultClient, err := c.GetVaultClient()
	if err != nil {
		return nil, errors.Wrap(err, "get vault client")
	}

	return &pkg.TemplateHelper{
		TemplateValues: c.GetTemplateArgs(),
		VaultClient:    vaultClient,
	}, nil
}

func (c BosunContext) WithEnv(env *EnvironmentConfig) BosunContext {
	c.Env = env
	c.Log = c.Log.WithField("env", env.Name)
	return c
}

func (c BosunContext) LogLine(skip int, format string, args ...interface{}) {
	if c.Log != nil {
		_, file, line, _ := runtime.Caller(skip)
		c.Log.WithField("loc", fmt.Sprintf("%s:%d", file, line)).Debugf(format, args...)
	}
}

var useMinikubeForDockerOnce = new(sync.Once)

func (c BosunContext) UseMinikubeForDockerIfAvailable() {
	useMinikubeForDockerOnce.Do(func() {
		if err := pkg.NewCommand("minikube", "ip").RunE(); err == nil {

		}
	})
}

func (c BosunContext) AddAppFileToReleaseBundle(path string, content []byte) (string, error) {
	app := c.AppRepo
	if app == nil {
		return "", errors.New("no app set in context")
	}
	release := c.Release
	if release == nil {
		return "", errors.New("no release set in context")
	}

	bundleFilePath := release.AddBundleFile(app.Name, path, content)
	return bundleFilePath, nil
}

func (c BosunContext) GetAppFileFromReleaseBundle(path string) ([]byte, string, error) {
	app := c.AppRepo
	if app == nil {
		return nil, "", errors.New("no app set in context")
	}
	release := c.Release
	if release == nil {
		return nil, "", errors.New("no release set in context")
	}

	content, bundleFilePath, err := release.GetBundleFileContent(app.Namespace, path)
	return content, bundleFilePath, err
}

func (c BosunContext) IsVerbose() bool {
	return c.GetParams().Verbose
}

func (c BosunContext) GetDomain() string {
	if c.Env != nil {
		return c.Env.Domain
	}
	return ""
}
func (c BosunContext) GetCluster() string {
	if c.Env != nil {
		return c.Env.Cluster
	}
	return ""
}
