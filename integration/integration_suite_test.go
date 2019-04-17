package integration_test

import (
	. "github.com/naveego/bosun/pkg/bosun"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// RootPkgBosunDir points to github.com/naveego/bosun/pkg/bosun
var RootPkgBosunDir string

// RootPkgBosunDir points to github.com/naveego/bosun
var RootDir string

// RootPkgBosunDir points to github.com/naveego/bosun/test
var IntegrationTestDir string

var TestDataDir string

func TestBosun(t *testing.T) {
	RegisterFailHandler(Fail)
	RootPkgBosunDir, _ = os.Getwd()
	RootDir = filepath.Join(RootPkgBosunDir, "../")
	IntegrationTestDir = filepath.Join(RootDir, "integration")
	TestDataDir = filepath.Join(IntegrationTestDir, "testdata")

	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		ForceColors: true,
	})

	RunSpecs(t, "Bosun Suite")
}

func GetBosun(optionalParams ...Parameters) *Bosun {
	config, err := LoadWorkspace(filepath.Join(TestDataDir, "bosun.yaml"))
	Expect(err).ToNot(HaveOccurred())

	var params Parameters
	if len(optionalParams) > 0 {
		params = optionalParams[0]
	}

	if params.ValueOverrides == nil {
		params.ValueOverrides = map[string]string{}
	}

	b, err := New(params, config)
	Expect(err).ToNot(HaveOccurred())
	return b
}
