package testhelpers

import (
	"context"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/templating"
	"github.com/naveego/bosun/pkg/util"
	"github.com/sirupsen/logrus"
	"time"
)

type MockExecutionContext struct {
	Parameters cli.Parameters
	MockPwd string
	EnvironmentVariables map[string]string
	MockTemplateValues templating.TemplateValues
	Context context.Context
	MockLog *logrus.Entry
}

func NewMockExecutionContext() *MockExecutionContext {
	return &MockExecutionContext{
		EnvironmentVariables: map[string]string{},
	}
}

func (m *MockExecutionContext) GetParameters() cli.Parameters {
	return m.Parameters
}

func (m *MockExecutionContext) Pwd() string {
	return m.MockPwd
}

func (m *MockExecutionContext) WithPwd(pwd string) cli.WithPwder {
m.MockPwd = pwd
}

func (m *MockExecutionContext) GetEnvironmentVariables() map[string]string {
if m.EnvironmentVariables == nil {
	m.EnvironmentVariables = map[string]string{}
}
return m.EnvironmentVariables
}

func (m *MockExecutionContext) TemplateValues() templating.TemplateValues {
	return m.MockTemplateValues
}

func (m *MockExecutionContext) Log() *logrus.Entry {
	if m.MockLog == nil {
		m.MockLog = logrus.NewEntry(logrus.StandardLogger())
	}
	return m.MockLog
}

func (m *MockExecutionContext) WithLogField(name string, value interface{}) util.WithLogFielder {
	m.MockLog = m.Log().WithField(name, value)
	return m
}

func (m *MockExecutionContext) Ctx() context.Context {
	if m.Context == nil {
		m.Context = context.Background()
	}
	return m.Context
}

func (m *MockExecutionContext) WithTimeout(timeout time.Duration) core.Ctxer {
	m.Context, _ = context.WithTimeout(m.Ctx(), timeout)
	return m
}

