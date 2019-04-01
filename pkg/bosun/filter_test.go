package bosun_test

import (
	. "github.com/naveego/bosun/pkg/bosun"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = XDescribe("Filter", func() {

	var repos = []*AppRepo{
		{
			AppRepoConfig: &AppRepoConfig{
				Name: "app-0",
			},
		},
		{
			AppRepoConfig: &AppRepoConfig{
				Name: "app-1",
			},
		},
		{
			AppRepoConfig: &AppRepoConfig{
				Name: "app-2",
			},
		},
		{
			AppRepoConfig: &AppRepoConfig{
				Name: "app-3",
			},
		},
	}

	var reposMap = map[string]*AppRepo{
		"app-0": repos[0],
		"app-1": repos[1],
		"app-2": repos[2],
		"app-3": repos[3],
	}

	// var releases = []*AppRelease{
	// 	{
	// 		AppRepo:repos[0],
	//
	// 	},{
	// 		AppRepo:repos[1],
	//
	// 	},{
	// 		AppRepo:repos[2],
	//
	// 	},{
	// 		AppRepo:repos[3],
	//
	// 	},
	// }
	//
	// var releasesMap = map[string]*AppRelease{
	// 	"app-0": releases[0],
	// 	"app-1": releases[1],
	// 	"app-2": releases[2],
	// 	"app-3": releases[3],
	//
	// }

	It("should filter app repos in array by name", func() {
		actual := ApplyFilter(repos, true, FiltersFromNames("app-2", "app-3"))
		Expect(actual).To(BeEquivalentTo([]*AppRepo{repos[2], repos[3]}))
	})

	It("should filter app repos in map by name", func() {
		actual := ApplyFilter(reposMap, true, FiltersFromNames("app-2", "app-3"))
		Expect(actual).To(BeEquivalentTo(map[string]*AppRepo{"app-2":repos[2], "app-3":repos[3]}))
	})
})
