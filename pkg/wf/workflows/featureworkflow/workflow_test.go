package featureworkflow_test

import (
	"github.com/naveego/bosun/pkg/wf/testharness"
.	"github.com/naveego/bosun/pkg/wf/workflows/featureworkflow"
	. "github.com/onsi/ginkgo"
)

var _ = Describe("Workflow", func() {

	var harness *testharness.TestHarness

	BeforeEach(func() {
		harness = testharness.New()

		harness.Registry.Register(Type, New)
	})



})
