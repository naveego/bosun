package actions_test

import (
	. "github.com/naveego/bosun/pkg/actions"
	"github.com/naveego/bosun/pkg/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ActionSchedule", func() {

	It("should marshal correctly", func() {

		sut := &AppAction{
			When: ActionSchedules{ActionAfterDeploy},
		}

		y, err := yaml.MarshalString(sut)
		Expect(err).ToNot(HaveOccurred())

		var actual AppAction
		Expect(yaml.Unmarshal([]byte(y), &actual)).To(Succeed())

		Expect(actual.When).To(ConsistOf(ActionAfterDeploy))

	})

})
