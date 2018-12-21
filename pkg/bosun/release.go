package bosun

type Release struct {
	Name string `yaml:"name"`
	FromPath string `yaml:"fromPath"`
	Apps []AppRelease `yaml:"apps"`
}

type AppRelease struct {
	Name string `yaml:"name"`
	Repo string `yaml:"repo"`
	RepoPath string `yaml:"repoPath"`
	BosunPath string `yaml:"bosunPath"`
	Branch string `yaml:"branch"`
	Tag string `yaml:"tag"`
}



func (a *App) MakeAppRelease() (AppRelease, error) {

	r := AppRelease{
		Name:a.Name,
		BosunPath:a.FromPath,
	}


	return r, nil

}