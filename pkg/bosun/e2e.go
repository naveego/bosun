package bosun

import (
	"fmt"
	"github.com/naveego/bosun/pkg/mongo"
	"github.com/pkg/errors"
	"github.com/rs/xid"
	"path/filepath"
	"time"
)

type E2ESuiteConfig struct {
	ConfigShared      `yaml:",inline"`
	E2EBookendScripts `yaml:",inline"`
	MongoConnections  map[string]mongo.Connection `yaml:"mongoConnections,omitempty"`
	TestFiles         []string                    `yaml:"tests"`
	Tests             []*E2ETestConfig            `yaml:"-"`
}

type E2ETestConfig struct {
	ConfigShared      `yaml:",inline"`
	E2EBookendScripts `yaml:",inline"`
	Dependencies      []*Dependency          `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`
	Variables         map[string]interface{} `yaml:"variables,omitempty" json:"variables"`
	Steps             []ScriptStep           `yaml:"steps,omitempty" json:"steps,omitempty"`
}

type E2EContext struct {
	SkipSetup    bool
	SkipTeardown bool
	Tests        []string
}

const e2eContextKey = "e2e.context"

func WithE2EContext(ctx BosunContext, e2eCtx E2EContext) BosunContext {
	return ctx.WithKeyedValue(e2eContextKey, e2eCtx)
}

func GetE2EContext(ctx BosunContext) E2EContext {
	x := ctx.GetKeyedValue(e2eContextKey)
	if x != nil {
		return x.(E2EContext)
	}
	return E2EContext{}
}

func (e *E2ESuiteConfig) SetFromPath(path string) {
	e.FromPath = path
	e.SetupScript.SetFromPath(path)
	e.TeardownScript.SetFromPath(path)
	for _, t := range e.Tests {
		t.SetFromPath(path)
	}
}

func (e *E2ETestConfig) SetFromPath(path string) {
	e.FromPath = path
	e.SetupScript.SetFromPath(path)
	e.TeardownScript.SetFromPath(path)
	if e.TeardownScript != nil {
		e.TeardownScript.SetFromPath(path)
	}
}

type E2EStepConfig struct {
	ConfigShared `yaml:",inline"`
}

type E2ESuite struct {
	E2ESuiteConfig
	PreparedConnections []mongo.PreparedConnection
}

type E2EBookendScripts struct {
	SetupScript    *Script `yaml:"setupScript,omitempty" json:"setupScript"`
	TeardownScript *Script `yaml:"teardownScript,omitempty" json:"teardownScript,omitempty"`
}

func (e E2EBookendScripts) Setup(ctx BosunContext) error {
	if e.SetupScript == nil {
		ctx.Log.Debug("No setup script defined.")
		return nil
	}

	if GetE2EContext(ctx).SkipSetup {
		ctx.Log.Warn("Skipping setup because skip-teardown flag was set.")
		return nil
	}

	err := e.SetupScript.Execute(ctx)
	return err
}

func (e E2EBookendScripts) Teardown(ctx BosunContext) error {

	if e.TeardownScript == nil {
		ctx.Log.Debug("No teardown script defined.")
		return nil
	}

	if GetE2EContext(ctx).SkipTeardown {
		ctx.Log.Warn("Skipping teardown because skip-teardown flag was set.")
		return nil
	}

	err := e.TeardownScript.Execute(ctx)
	return err
}

func NewE2ESuite(ctx BosunContext, config *E2ESuiteConfig) (*E2ESuite, error) {

	suite := &E2ESuite{
		E2ESuiteConfig: *config,
	}

	return suite, nil
}

func (s *E2ESuite) LoadTests(ctx BosunContext) error {
	templateHelper, err := ctx.GetTemplateHelper()
	if err != nil {
		return errors.Wrap(err, "get template helper")
	}

	testDir := filepath.Dir(s.FromPath)

	for _, path := range s.TestFiles {
		path = filepath.Join(testDir, path)

		var testConfig E2ETestConfig
		err = templateHelper.LoadFromYaml(&testConfig, path)
		if err != nil {
			return errors.Wrapf(err, "read test config from %q", path)
		}

		testConfig.SetFromPath(path)
		s.Tests = append(s.Tests, &testConfig)
	}

	return nil
}

func (s *E2ESuite) Run(ctx BosunContext, tests ...string) ([]*E2EResult, error) {

	runID := xid.New().String()
	releaseValues := &ReleaseValues{
		Values: Values{
			"e2e": Values{
				"runID": runID,
			},
		},
	}

	ctx = ctx.WithDir(s.FromPath).
		WithLogField("suite", s.Name).
		WithReleaseValues(releaseValues)

	err := s.LoadTests(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "load tests")
	}

	run := &E2ERun{
		ID:    runID,
		Ctx:   ctx,
		Suite: s,
	}

	defer func() {
		for _, p := range s.PreparedConnections {
			p.CleanUp()
		}
	}()

	// populate context with mongo connections needed by scripts
	for k, v := range s.MongoConnections {
		// prepare the connection so that we establish a port forwarder (if needed)
		// that will live for lifetime of the suite
		p, err := v.Prepare(ctx.Log)
		if err != nil {
			return nil, errors.Wrapf(err, "could not prepare suite mongo connection %q", k)
		}
		s.PreparedConnections = append(s.PreparedConnections, p)
		run.Ctx = run.Ctx.WithKeyedValue(k, v)
	}

	// get the configs which should be in this run
	if len(tests) == 0 {
		run.Configs = s.Tests
	} else {
		for _, name := range tests {
			for _, test := range s.Tests {
				if test.Name == name {
					run.Configs = append(run.Configs, test)
				}
			}
		}
	}

	// create the tests which will be run
	for _, testConfig := range run.Configs {
		run.Tests = append(run.Tests, NewE2ETest(*testConfig))
	}

	// run the tests
	err = run.Execute()

	return run.Results, err

}

func NewE2ETest(config E2ETestConfig) *E2ETest {
	return &E2ETest{
		E2ETestConfig: config,
	}
}

type E2ERun struct {
	ID      string
	Ctx     BosunContext
	Suite   *E2ESuite
	Configs []*E2ETestConfig
	Tests   []*E2ETest
	Results []*E2EResult
}

func (r *E2ERun) Execute() error {

	// error out if the suit setup fails
	if err := r.Suite.Setup(r.Ctx); err != nil {
		return errors.Wrap(err, "suite setup")

	}

	// log if the suite teardown fails, but don't error out
	defer func() {
		if err := r.Suite.Teardown(r.Ctx); err != nil {
			r.Ctx.Log.WithError(err).Error("Error during teardown.")
		}
	}()

	// execute all the tests
	// right now we stop running if a test fails, we may want to change that in the future
	for _, test := range r.Tests {
		result, err := test.Execute(r.Ctx)
		if err != nil {
			return errors.Wrapf(err, "test %q errored", test.Name)
		}

		r.Results = append(r.Results, result)
	}

	return nil
}

type E2ETest struct {
	E2ETestConfig
}

type Timed struct {
	StartedAt time.Time `yaml:"startedAt" json:"startedAt"`
	EndedAt   time.Time `yaml:"endedAt" json:"endedAt"`
	Elapsed   string    `yaml:"elapsed" json:"elapsed"`
}

type E2EResult struct {
	Name   string           `yaml:"name" json:"name"`
	Steps  []*E2EStepResult `yaml:"steps" json:"steps"`
	Passed bool             `yaml:"passed" json:"passed"`
	Error  string           `yaml:"error,omitempty" json:"error,omitempty"`
	Timed  `yaml:",inline"`
}

type E2EStepResult struct {
	Name   string `yaml:"name" json:"name"`
	Passed bool   `yaml:"passed" json:"passed"`
	Error  string `yaml:"error,omitempty" json:"error,omitempty"`
	Timed  `yaml:",inline"`
}

func (t *Timed) StartTimer() {
	t.StartedAt = time.Now()
}
func (t *Timed) StopTimer() {
	t.EndedAt = time.Now()
	elapsed := t.EndedAt.Sub(t.StartedAt)
	t.Elapsed = elapsed.String()
}

func (e *E2ETest) Execute(ctx BosunContext) (*E2EResult, error) {
	ctx = ctx.WithDir(e.FromPath).WithLog(ctx.Log.WithField("test", e.Name))

	result := &E2EResult{
		Name:   e.Name,
		Passed: true,
	}

	result.StartTimer()
	defer func(result *E2EResult) {
		if err := e.Teardown(ctx); err != nil {
			ctx.Log.WithError(err).Error("Error during teardown.")
		}
		result.StopTimer()
	}(result)

	if err := e.Setup(ctx); err != nil {
		result.Passed = false
		return result, errors.Wrap(err, "test setup")
	}

	for i, step := range e.Steps {

		stepResult := &E2EStepResult{
			Name:   step.Name,
			Passed: true,
		}
		if stepResult.Name == "" {
			stepResult.Name = fmt.Sprint(i)
		}

		stepResult.StartTimer()

		err := step.Execute(ctx, i)
		stepResult.StopTimer()
		result.Steps = append(result.Steps, stepResult)

		if err != nil {
			ctx.Log.WithError(err).Error("Step failed.")
			result.Passed = false
			result.Error = err.Error()
			stepResult.Passed = false
			stepResult.Error = err.Error()
			break
		}
	}

	return result, nil
}
