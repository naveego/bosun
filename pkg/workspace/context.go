package workspace

type Context struct {
	CurrentEnvironment string                `yaml:"currentEnvironment" json:"currentEnvironment"`
	CurrentPlatform    string                `yaml:"currentPlatform" json:"currentPlatform"`
	CurrentRelease     string                `yaml:"currentRelease" json:"currentRelease"`
	CurrentCluster     string                `yaml:"currentCluster" json:"currentCluster"`
	CurrentKubeconfig  string                `yaml:"currentKubeconfig" json:"currentKubeconfig"`
	CurrentStack       string                `yaml:"currentStack" json:"currentStack"`
	KnownStacks        map[string]KnownStack `yaml:"knownStacks"`
}

type KnownStack struct {
	Environment  string
	Cluster      string
	Name         string
	TemplateName string
}

type Contexter interface {
	WorkspaceContext() Context
}
