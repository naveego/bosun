package bosun_test

import (
	. "github.com/naveego/bosun/pkg/bosun"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("AppRepo", func() {

	It("should support topological sort", func() {

		apps := map[string]*AppRepo{
			"a": {
				AppRepoConfig: &AppRepoConfig{
					Name: "a",
					DependsOn: []Dependency{
						{Name: "b"},
						{Name: "c"},
					},
				},
			},
			"b": {
				AppRepoConfig: &AppRepoConfig{
					Name: "b",
				},
			},
			"c": {
				AppRepoConfig: &AppRepoConfig{
					Name: "c",
					DependsOn: []Dependency{
						{Name: "b"},
					},
				},
			},
		}

		sorted, err := GetDependenciesInTopologicalOrder(apps, "a")
		Expect(err).ToNot(HaveOccurred())
		Expect(sorted).To(HaveLen(3))

		var names []string
		for _, app := range sorted {
			names = append(names, app.Name)
		}

		Expect(names).To(BeEquivalentTo([]string{"b", "c", "a"}))
	})
})
