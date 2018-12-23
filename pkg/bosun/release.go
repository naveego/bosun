package bosun

type Release struct {
	Name     string       `yaml:"name"`
	FromPath string       `yaml:"fromPath"`
	Apps     map[string]AppRelease `yaml:"apps"`
	Fragment *ConfigFragment `yaml:"-"`
}

func (r *Release) SetFragment(f *ConfigFragment) {
	r.FromPath = f.FromPath
	r.Fragment = f
}

type AppRelease struct {
	Name string `yaml:"name"`
	Repo string `yaml:"repo"`
	RepoPath string `yaml:"repoPath"`
	BosunPath string `yaml:"bosunPath"`
	Branch string `yaml:"branch"`
	Version string `yaml:"version"`
	Tag string `yaml:"tag"`
	Commit string `yaml:"string"`
	App *App `yaml:"-"`
}



func (a *App) MakeAppRelease() (AppRelease, error) {

	r := AppRelease{
		Name:a.Name,
		BosunPath:a.FromPath,
	}


	return r, nil

}