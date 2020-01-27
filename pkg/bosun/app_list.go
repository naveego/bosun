package bosun

import (
	"github.com/naveego/bosun/pkg/util/multierr"
	"go4.org/sort"
	"sync"
)

type AppList []*App
type AppMap map[string]*App

func (a AppMap) ToList() AppList {
	out := AppList{}
	for _, app := range a {
		out = append(out, app)
	}
	return out
}

func (a AppList) ToMap() AppMap {
	out := AppMap{}
	for _, app := range a {
		out[app.Name] = app
	}
	return out
}

func (a AppList) SortByProvider() AppList {
	sort.With(len(a),
		func(i, j int) {
			a[i], a[j] = a[j], a[i]
		}, func(i, j int) bool {
			return a[i].Provider.String() < a[j].Provider.String()
		})
	return a
}

func (a AppList) SortByName() AppList {
	sort.With(len(a),
		func(i, j int) {
			a[i], a[j] = a[j], a[i]
		}, func(i, j int) bool {
			return a[i].Name < a[j].Name
		})
	return a
}

func (a AppList) Sort(less func(a, b *App) bool) AppList {
	sort.With(len(a),
		func(i, j int) {
			a[i], a[j] = a[j], a[i]
		}, func(i, j int) bool {
			return less(a[i], a[j])
		})
	return a
}

func (a AppList) Map(fn func(a *App) interface{}) []interface{} {
	out := make([]interface{}, 0, len(a))
	for i := range a {
		o := fn(a[i])
		out = append(out, o)
	}
	return out
}

func (a AppList) FlatMap(fn func(a *App) []interface{}) []interface{} {
	out := make([]interface{}, 0, len(a))
	for _, app := range a {
		o2 := fn(app)
		for _, o := range o2 {
			out = append(out, o)
		}
	}
	return out
}

func (a AppList) Filter(fn func(a *App) bool) AppList {
	out := AppList{}
	for _, app := range a {
		if fn(app) {
			out = append(out, app)
		}
	}
	return out
}

func (a AppList) GroupByAppThenProvider() map[string]map[string]*App {
	var out = map[string]map[string]*App{}
	for _, app := range a {
		byProvider, ok := out[app.Name]
		if !ok {
			byProvider = map[string]*App{}
			out[app.Name] = byProvider
		}
		byProvider[app.ProviderName()] = app
	}
	return out
}

func (a AppList) FetchAll(logger Logger) error {
	return a.ForEachRepo(func(app *App) error {
		return app.Repo.Fetch()
	})
}

func (a AppList) CheckOutDevelop(logger Logger) error {
	return a.ForEachRepo(func(app *App) error {
		return app.Repo.LocalRepo.SwitchToBranchAndPull(logger, app.Branching.Develop)
	})
}

func (a AppList) ForEachRepo(fn func(app *App) error) error {
	errs := multierr.New()
	wg := new(sync.WaitGroup)
	queued := map[string]bool{}

	for _, app := range a {
		if app.CheckRepoCloned() == nil {
			if queued[app.RepoName] {
				continue
			}
			queued[app.RepoName] = true
			wg.Add(1)
			go func(app *App) {
				defer wg.Done()
				err := fn(app)
				if err != nil {
					errs.Collect(err)
				}

			}(app)
		}
	}
	wg.Wait()
	return errs.ToError()
}
