package workspace

type Context struct {
	CurrentEnvironment string `yaml:"currentEnvironment" json:"currentEnvironment"`
	CurrentPlatform    string `yaml:"currentPlatform" json:"currentPlatform"`
	CurrentRelease     string `yaml:"currentRelease" json:"currentRelease"`
	CurrentCluster     string `yaml:"currentCluster" json:"currentCluster"`
	CurrentKubeconfig  string `yaml:"currentKubeconfig" json:"currentKubeconfig"`
	CurrentStack       string `yaml:"currentSubcluster" json:"currentSubcluster"`
}

type Contexter interface {
	WorkspaceContext() Context
}
