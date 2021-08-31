package bosun

import (
	"fmt"
	"github.com/kyokomi/emoji"
	"github.com/naveego/bosun/pkg/util"
	"github.com/naveego/bosun/pkg/util/multierr"
	"go4.org/sort"
	"strings"
	"sync"
)

type AppList []*App

func (a AppList) Headers() []string {

	return []string{
		AppListColName,
		AppListColPath,
		AppListColCloned,
		AppListColVersion,
		AppListColRepo,
		AppListColBranch,
		AppListColDirty,
		AppListColStale,
		AppListColImages,
		AppListColLabels,
	}
}

func (a AppList) Rows() [][]string {

	var out [][]string
	cols := a.Headers()

	for _, app := range a {
		m := map[string]string{}

		m[AppListColName] = app.Name
		m[AppListColRepo] = app.RepoName

		if app.IsRepoCloned() {
			m[AppListColCloned] = fmtBool(true)
			m[AppListColBranch] = app.GetBranchName().String()
			m[AppListColVersion] = app.Version.String()

			m[AppListColDirty] = fmtBool(app.Repo.LocalRepo.IsDirty())

			m[AppListColStale] = app.Repo.LocalRepo.GetUpstreamStatus()
		} else {
			m[AppListColCloned] = fmtBool(false)
			m[AppListColVersion] = app.Version.String()
		}

		m[AppListColPath] = app.AppConfig.FromPath

		var labelLines []string
		for _, k := range util.SortedKeys(app.Labels) {
			labelLines = append(labelLines, fmt.Sprintf("%s: %s", k, app.Labels[k]))
		}
		m[AppListColLabels] = strings.Join(labelLines, "\n")

		var imageLines []string
		images := app.GetImages()
		for _, image := range images {
			imageLines = append(imageLines, image.GetFullName())
		}
		m[AppListColImages] = strings.Join(imageLines, "\n")

		var row []string

		for _, c := range cols {
			row = append(row, m[c])
		}

		out = append(out, row)
	}

	return out

}

func fmtBool(b bool) string {
	if b {
		return emoji.Sprint("YES    ")
	} else {
		return emoji.Sprint("     NO")
	}
}

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

const (
	AppListColName    = "app"
	AppListColCloned  = "cloned"
	AppListColVersion = "version"
	AppListColRepo    = "repo"
	AppListColBranch  = "branch"
	AppListColDirty   = "dirty"
	AppListColStale   = "stale"
	AppListColPath    = "path"
	AppListColImages  = "images"
	AppListColLabels  = "meta-labels"
)
