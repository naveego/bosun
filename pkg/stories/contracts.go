package stories

type Story struct {
	// Identifier string which the provider can understand
	ID            string   `yaml:"id,omitempty"`
	// URL link which can be shown to users
	URL           string   `yaml:"url,omitempty"`
	// Reference string which can be embedded in an issue
	Reference     string   `yaml:"reference,omitempty"`

	Body          string   `yaml:"body,omitempty"`
	Assignee      string   `yaml:"assignee,omitempty"`
	Assignees     []string `yaml:"assignees,omitempty"`
	Milestone     string   `yaml:"milestone,omitempty"`
	Estimate      string   `yaml:"estimate,omitempty"`
	Epics         []string `yaml:"epics,omitempty"`
	Title         string   `yaml:"title,omitempty"`
	ProgressState string   `yaml:"progressState,omitempty"`
	Labels        []string `yaml:"labels,omitempty"`
	IsClosed      bool     `yaml:"isClosed,omitempty"`
	// Metadata about the story that is provider specific
	ProviderState interface{} `yaml:"providerState,omitempty"`
}

type Dependency struct {
	// Identifier string which the provider can understand
	ID            string   `yaml:"id,omitempty"`
	// URL link which can be shown to users
	URL           string   `yaml:"url,omitempty"`
	// Reference string which can be embedded in an issue
	Reference     string   `yaml:"reference,omitempty"`
	// Display name for the dependency
	Title string `yaml:"title,omitempty"`
}


type StoryClient interface {

	GetStory(id string) (*Story, error)
	AssignStory(story *Story, users []string) (*Story, error)
	AddDependency(story *Story, dependency Dependency) (*Story, error)
	SetStoryProgress(story *Story, progress string) (*Story, error)

}