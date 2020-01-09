package bosun

import (
	"context"
	"fmt"
	"github.com/fatih/color"
	vault "github.com/hashicorp/vault/api"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/environment"
	"github.com/naveego/bosun/pkg/ioc"
	"github.com/naveego/bosun/pkg/kube"
	"github.com/naveego/bosun/pkg/templating"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/workspace"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"time"
)

type BosunContext struct {
	Bosun  *Bosun
	env    *environment.Environment
	Dir    string
	log    *logrus.Entry
	Values *values.PersistableValues
	// Release         *Deploy
	appRepo          *App
	appRelease       *AppDeploy
	ctx              context.Context
	contextValues    map[string]interface{}
	provider         ioc.Provider
	workspaceContext *workspace.Context
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
		env:   &environment.Environment{},
		Bosun: testBosun,
		log:   logrus.WithField("TEST", "INSTANCE"),
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

func (c BosunContext) WithValue(key string, value interface{}) core.StringKeyValuer {
	c.LogLine(1, "[Context] Set value at %q to %T", key, value)
	kvs := map[string]interface{}{}
	for k, v := range c.contextValues {
		kvs[k] = v
	}
	kvs[key] = value
	c.contextValues = kvs
	return c
}

func (c BosunContext) GetValue(key string, defaultValue ...interface{}) interface{} {
	if out, ok := c.contextValues[key]; ok {
		return out
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	panic(fmt.Sprintf("no value with key %q", key))
}

func (c BosunContext) WorkspaceContext() workspace.Context {
	if c.workspaceContext != nil {
		return *c.workspaceContext
	}
	return c.Bosun.ws.Context
}

func (c BosunContext) WithWorkspaceContext(ctx workspace.Context) BosunContext {
	c.workspaceContext = &ctx
	return c
}

//
// func (c BosunContext) WithRelease(r *Deploy) BosunContext {
// 	c.Release = r
// 	c.Log = c.Log.WithField("release", r.Name)
// 	c.LogLine(1, "[Context] Changed Deploy.")
// 	return c
// }

func (c BosunContext) WithApp(a *App) BosunContext {
	if c.appRepo == a {
		return c
	}
	c.appRepo = a
	c.log = c.Log().WithField("app", a.Name)
	c.Log().Debug("")
	c.LogLine(1, "[Context] Changed App.")
	return c.WithDir(a.FromPath)
}

func (c BosunContext) WithAppDeploy(a *AppDeploy) BosunContext {
	if c.appRelease == a {
		return c
	}
	c.appRelease = a
	c.log = c.Log().WithField("appDeploy", a.Name)
	c.LogLine(1, "[Context] Changed AppDeploy.")
	return c.WithDir(a.AppConfig.FromPath)
}

func (c BosunContext) WithPersistableValues(v *values.PersistableValues) interface{} {
	c.Values = v

	yml, _ := yaml.Marshal(v.Values)
	c.LogLine(1, "[Context] Set release values:\n%s\n", string(yml))

	return c
}

func (c BosunContext) GetEnvironmentVariables() map[string]string {

	out := map[string]string{
		core.EnvCluster:         c.env.ClusterName,
		core.EnvEnvironment:     c.env.Name,
		core.EnvEnvironmentRole: string(c.env.Role),
	}

	envValues := c.env.ValueSet.Static.ToEnv("BOSUN_")

	for k, v := range envValues {
		out[k] = v
	}

	return out
}

func (c BosunContext) WithLog(log *logrus.Entry) BosunContext {
	c.log = log
	return c
}

func (c BosunContext) WithLogField(key string, value interface{}) util.WithLogFielder {
	c.log = c.Log().WithField(key, value)
	return c
}

func (c BosunContext) GetVaultClient() (*vault.Client, error) {
	return c.Bosun.GetVaultClient()
}

func (c BosunContext) WithContext(ctx context.Context) BosunContext {
	c.ctx = ctx
	return c
}

func (c BosunContext) WithTimeout(timeout time.Duration) core.Ctxer {
	ctx, _ := context.WithTimeout(c.Ctx(), timeout)
	return c.WithContext(ctx)
}

// ResolvePath resolves the path relative to the Dir in this context.
// It will also expand some environment variables:
// $ENVIRONMENT and $BOSUN_ENVIRONMENT => Env.Name
// $DOMAIN AND BOSUN_DOMAIN => Env.Domain
// It will also include any additional expansions provided.
func (c BosunContext) ResolvePath(path string, expansions ...string) string {

	expMap := util.StringSliceToMap(expansions...)
	path = os.Expand(path, func(name string) string {
		switch name {
		case "ENVIRONMENT", core.EnvEnvironment:
			return c.env.Name
		case core.EnvEnvironmentRole:
			return string(c.env.Role)
		default:
			if v, ok := expMap[name]; ok {
				return v
			}
			return name
		}
	})

	if !filepath.IsAbs(path) {
		path = filepath.Join(c.Dir, path)
	}
	return path
}

func (c BosunContext) GetParameters() cli.Parameters {
	if c.Bosun != nil {
		return c.Bosun.params
	}
	return cli.Parameters{}
}

func (c BosunContext) TemplateValues() templating.TemplateValues {
	tv := templating.TemplateValues{
		Cluster: c.WorkspaceContext().CurrentCluster,
	}
	if c.Values != nil {
		values := c.Values.Values
		values.MustSetAtPath("cluster", c.WorkspaceContext().CurrentCluster)
		tv.Values = values
	} else {
		tv.Values = values.Values{}
	}

	return tv
}

func (c BosunContext) GetTemplateHelper() (*pkg.TemplateHelper, error) {
	vaultClient, err := c.GetVaultClient()
	if err != nil {
		return nil, errors.Wrap(err, "get vault client")
	}

	return &pkg.TemplateHelper{
		TemplateValues: c.TemplateValues(),
		VaultClient:    vaultClient,
	}, nil
}

func (c BosunContext) WithEnv(env environment.Environment) BosunContext {
	c.env = &env
	c.log = c.Log().WithField("env", env.Name)
	return c
}

func (c BosunContext) LogLine(skip int, format string, args ...interface{}) {
	if c.Log() != nil {
		_, file, line, _ := runtime.Caller(skip)
		c.Log().WithField("loc", fmt.Sprintf("%s:%d", file, line)).Debugf(format, args...)
	}
}

var useMinikubeForDockerOnce = new(sync.Once)
var dockerEnv []string

func (c BosunContext) GetMinikubeDockerEnv() []string {
	useMinikubeForDockerOnce.Do(func() {
		defer func() {
			e := recover()
			if e != nil {
				color.Red("Attempting to use docker for minikube panicked: %v", e)
			}
		}()
		log := c.Log()
		log.Info("Attempting to use docker agent in minikube...")
		if err := pkg.NewShellExe("minikube", "ip").RunE(); err != nil {
			log.Warnf("Could not use minikube as a docker proxy: %s", err)
			return
		}

		envblob, err := pkg.NewShellExe("minikube", "docker-env").RunOut()
		if err != nil {
			log.WithError(err).Error("Could not get docker-env.")
			return
		}
		envs := regexp.MustCompile(`([A-Z_]+)="([^"])"`).FindAllStringSubmatch(envblob, -1)
		for _, env := range envs {
			log.Debugf("Setting env %s=%s", env[0], env[1])
			_ = os.Setenv(env[0], env[1])
			dockerEnv = append(dockerEnv, fmt.Sprintf("%s=%s", env[0], env[1]))
		}
		log.Info("Minikube docker agent configured.")
	})

	return dockerEnv
}

//
// func (c BosunContext) AddAppFileToReleaseBundle(path string, content []byte) (string, error) {
// 	app := c.AppRepo
// 	if app == nil {
// 		return "", errors.New("no app set in context")
// 	}
// 	release := c.Release
// 	if release == nil {
// 		return "", errors.New("no release set in context")
// 	}
//
// 	bundleFilePath := release.AddBundleFile(app.Name, path, content)
// 	return bundleFilePath, nil
// }
//
// func (c BosunContext) GetAppFileFromReleaseBundle(path string) ([]byte, string, error) {
// 	app := c.AppRepo
// 	if app == nil {
// 		return nil, "", errors.New("no app set in context")
// 	}
// 	release := c.Release
// 	if release == nil {
// 		return nil, "", errors.New("no release set in context")
// 	}
//
// 	content, bundleFilePath, err := release.GetBundleFileContent(app.Namespace, path)
// 	return content, bundleFilePath, err
// }

func (c BosunContext) IsVerbose() bool {
	return c.GetParameters().Verbose
}

func (c BosunContext) GetCluster() string {
	if c.env == nil {
		return c.WorkspaceContext().CurrentCluster
	}
	return c.env.ClusterName
}

// Log gets a logger safely.
func (c BosunContext) Log() *logrus.Entry {
	if c.log != nil {
		return c.log
	}
	return logrus.NewEntry(logrus.StandardLogger())
}

func (c BosunContext) Pwd() string {
	if c.Dir != "" {
		return c.Dir
	}
	pwd, _ := os.Getwd()
	return pwd
}

func (c BosunContext) WithPwd(pwd string) cli.WithPwder {
	return c.WithDir(pwd)
}

func (c BosunContext) WithStringValue(key string, value string) core.StringKeyValuer {
	if c.contextValues == nil {
		c.contextValues = map[string]interface{}{}
	}
	c.contextValues[key] = value
	return c
}

func (c BosunContext) GetStringValue(key string, defaultValue ...string) string {
	switch key {
	case core.KeyEnv:
		return c.env.Name
	case core.KeyCluster:
		return c.GetCluster()
	}

	if out, ok := c.contextValues[key].(string); ok {
		return out
	}

	if len(defaultValue) > 0 {
		return defaultValue[0]
	}

	panic(fmt.Sprintf("no value in context under key %q", key))
}

func (c BosunContext) Provide(out interface{}, options ...ioc.Options) error {
	if c.provider == nil {
		var container = ioc.NewContainer()
		container.BindFactory(c.GetVaultClient)

		container.BindFactory(kube.GetKubeClient)

		c.provider = container
	}

	return c.provider.Provide(out, options...)
}

func (c BosunContext) EnsureEnvironment() error {
	return c.env.Ensure(c)
}

func (c BosunContext) Environment() *environment.Environment {
	return c.env
}

func (c BosunContext) GetReleaseValues() *values.PersistableValues {
	return c.Values
}

func (c BosunContext) GetWorkspaceCommand(name string) *command.CommandValue {
	return c.Bosun.ws.GetWorkspaceCommand(name)
}
