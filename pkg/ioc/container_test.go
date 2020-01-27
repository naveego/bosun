package ioc_test

import (
	. "github.com/naveego/bosun/pkg/ioc"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type Dep1 struct {
	Value string
}

func (d Dep1) GetValue() string {
	return d.Value
}

type Interface1 interface {
	GetValue() string
}

var _ = Describe("Container", func() {
	Describe("when singleton is bound", func() {

		Describe("as default", func() {
			var sut *Container
			var expected Dep1

			BeforeEach(func() {
				sut = NewContainer()
				expected = Dep1{
					Value: "expected",
				}
				sut.BindSingleton(expected)
			})

			It("should provide instance", func() {
				actual := Dep1{}

				Expect(sut.Provide(&actual)).To(Succeed())
				Expect(actual).To(Equal(expected))
			})
		})

		Describe("as default to interface", func() {
			var sut *Container
			var expected Dep1

			BeforeEach(func() {
				sut = NewContainer()
				expected = Dep1{
					Value: "expected",
				}
				sut.BindSingleton(expected, Option().ProvidingTypes((*Interface1)(nil)))
			})

			It("should provide instance", func() {
				var actual Interface1

				Expect(sut.Provide(&actual)).To(Succeed())
				Expect(actual.GetValue()).To(Equal(expected.Value))
			})
		})

		Describe("as default pointer", func() {
			var sut *Container
			var expected *Dep1

			BeforeEach(func() {
				sut = NewContainer()
				expected = &Dep1{
					Value: "expected",
				}
				sut.BindSingleton(expected)
			})

			It("should provide instance", func() {
				actual := &Dep1{}

				Expect(sut.Provide(&actual)).To(Succeed())
				Expect(actual).To(Equal(expected))
			})
		})

		Describe("as named", func() {
			var sut *Container
			var expected Dep1
			var unexpected Dep1

			BeforeEach(func() {
				sut = NewContainer()
				expected = Dep1{
					Value: "expected",
				}
				unexpected = Dep1{
					Value: "unexpected",
				}

				sut.BindSingleton(unexpected)
				sut.BindSingleton(expected, Option().WithName("expected"))
			})

			It("should provide instance", func() {
				actual := Dep1{}

				Expect(sut.Provide(&actual, Option().WithName("expected"))).To(Succeed())
				Expect(actual).To(Equal(expected))
			})
		})
	})

	Describe("when factory is bound", func() {

		Describe("as default", func() {
			var sut *Container
			var expected Dep1

			BeforeEach(func() {
				sut = NewContainer()
				expected = Dep1{
					Value: "expected",
				}
				sut.BindFactory(func() Dep1 {
					return expected
				})
			})

			It("should provide instance", func() {
				actual := Dep1{}

				Expect(sut.Provide(&actual)).To(Succeed())
				Expect(actual).To(Equal(expected))
			})
		})

		Describe("as default to interface", func() {
			var sut *Container
			var expected Dep1

			BeforeEach(func() {
				sut = NewContainer()
				expected = Dep1{
					Value: "expected",
				}
				sut.BindFactory(func() Dep1 {
					return expected
				}, Option().ProvidingTypes((*Interface1)(nil)))
			})

			It("should provide instance", func() {
				var actual Interface1

				Expect(sut.Provide(&actual)).To(Succeed())
				Expect(actual.GetValue()).To(Equal(expected.Value))
			})
		})

		Describe("as factory taking provider", func() {
			var sut *Container
			var expected Dep1

			BeforeEach(func() {
				sut = NewContainer()
				expected = Dep1{
					Value: "expected",
				}

				sut.BindFactory(func(p Provider) Dep1 {
					return expected
				})
			})

			It("should provide instance", func() {
				actual := Dep1{}

				Expect(sut.Provide(&actual)).To(Succeed())
				Expect(actual).To(Equal(expected))
			})
		})
	})

})
