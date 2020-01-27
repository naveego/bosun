package actions_test

import (
	"context"
	"github.com/naveego/bosun/pkg/actions"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/ioc"
	"github.com/naveego/bosun/pkg/templating"
	"github.com/naveego/bosun/pkg/util"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// RootPkgBosunDir points to github.com/naveego/bosun/pkg/bosun
var RootPkgBosunDir string

// RootPkgBosunDir points to github.com/naveego/bosun
var RootDir string

// RootPkgBosunDir points to github.com/naveego/bosun/integration
var IntegrationTestDir string

func TestBosun(t *testing.T) {
	RegisterFailHandler(Fail)
	RootPkgBosunDir, _ = os.Getwd()
	RootDir = filepath.Join(RootPkgBosunDir, "../../")
	IntegrationTestDir = filepath.Join(RootDir, "integration")
	RunSpecs(t, "Actions Suite")
}

func NewTestActionContext() actions.ActionContext {

	return TestActionContext{
		log: logrus.NewEntry(logrus.StandardLogger()),
	}
}

type TestActionContext struct {
	log *logrus.Entry
}

func (t TestActionContext) ResolvePath(path string, expansions ...string) string {
	panic("implement me")
}

func (t TestActionContext) GetStringValue(key string, defaultValue ...string) string {
	panic("implement me")
}

func (t TestActionContext) WithStringValue(key string, value string) core.StringKeyValuer {
	panic("implement me")
}

func (t TestActionContext) GetValue(key string, defaultValue ...interface{}) interface{} {
	panic("implement me")
}

func (t TestActionContext) WithValue(key string, value interface{}) core.StringKeyValuer {
	panic("implement me")
}

func (t TestActionContext) Provide(instance interface{}, options ...ioc.Options) error {
	panic("implement me")
}

func (t TestActionContext) GetParameters() cli.Parameters {
	panic("implement me")
}

func (t TestActionContext) Pwd() string {
	panic("implement me")
}

func (t TestActionContext) WithPwd(pwd string) cli.WithPwder {
	panic("implement me")
}

func (t TestActionContext) GetEnvironmentVariables() map[string]string {
	panic("implement me")
}

func (t TestActionContext) TemplateValues() templating.TemplateValues {
	panic("implement me")
}

func (t TestActionContext) Log() *logrus.Entry {
	return t.log
}

func (t TestActionContext) WithLogField(name string, value interface{}) util.WithLogFielder {
	t.log = t.log.WithField(name, value)
	return t
}

func (t TestActionContext) Ctx() context.Context {
	panic("implement me")
}

func (t TestActionContext) WithTimeout(timeout time.Duration) core.Ctxer {
	panic("implement me")
}
