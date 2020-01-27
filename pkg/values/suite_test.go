package values_test

import (
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

// RootPkgBosunDir points to github.com/naveego/bosun/integration
var IntegrationTestDir string

func TestBosun(t *testing.T) {
	RegisterFailHandler(Fail)
	RootPkgBosunDir, _ = os.Getwd()
	RootDir = filepath.Join(RootPkgBosunDir, "../../")
	IntegrationTestDir = filepath.Join(RootDir, "integration")
	RunSpecs(t, "Values Suite")
}
