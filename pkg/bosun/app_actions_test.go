package bosun_test

import (
	. "github.com/naveego/bosun/pkg/bosun"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("AppActions", func() {

	It("should assemble actions by reflection", func() {

		sut := &AppAction{
			HTTP: &HTTPAction{},
		}

		actions := sut.GetActions()
		Expect(actions).To(HaveLen(1))

		sut = &AppAction{
			HTTP:  &HTTPAction{},
			Bosun: &BosunAction{"test"},
		}

		actions = sut.GetActions()
		Expect(actions).To(HaveLen(2))

		scriptAction := ScriptAction("some script")
		sut = &AppAction{
			HTTP:   &HTTPAction{},
			Bosun:  &BosunAction{"test"},
			Script: &scriptAction,
		}

		actions = sut.GetActions()
		Expect(actions).To(HaveLen(3))

	})

	Describe("HTTPAction", func() {
		It("should execute request", func() {

			sut := HTTPAction{
				Method: "GET",
				URL:    "https://google.com",
			}

			ctx := NewTestBosunContext()

			Expect(sut.Execute(ctx)).To(Succeed())
		})
	})
})
