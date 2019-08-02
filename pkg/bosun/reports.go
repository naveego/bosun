package bosun

type RepoInfo struct {
	App     string `yaml:"app" json:"app"`
	Name    string `yaml:"name" json:"name"`
	Path    string `yaml:"path" json:"path"`
	IsDirty bool   `yaml:"isDirty" json:"isDirty"`
	Branch  string `yaml:"Branch" json:"branch"`
}

func (b *Bosun) GetRepoInfo() ([]RepoInfo, error) {
	apps, err := b.GetAllVersionsOfAllApps(WorkspaceProviderName)
	if err != nil {
		return nil, err
	}

	var out []RepoInfo

	apps.SortByName().Map(func(app *App) interface{} {
		info := RepoInfo{
			App:     app.Name,
			Name:    app.Repo.LocalRepo.Name,
			Path:    app.Repo.LocalRepo.Path,
			Branch:  app.Repo.LocalRepo.GetCurrentBranch(),
			IsDirty: app.Repo.LocalRepo.IsDirty(),
		}
		out = append(out, info)
		return info
	})

	return out, err
}
