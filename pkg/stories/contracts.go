package stories

type Story struct {
	// Identifier string which the provider can understand
	ID            string   `yaml:"id,omitempty"`
	Title         string   `yaml:"title,omitempty"`
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
	ProgressState string   `yaml:"progressState,omitempty"`
	Labels        []string `yaml:"labels,omitempty"`
	IsClosed      bool     `yaml:"isClosed,omitempty"`
	// Metadata about the story that is provider specific
	ProviderState interface{} `yaml:"-"`
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




type StoryHandler interface {

	GetStory(id string) (*Story, error)

	HandleEvent(event *ValidatedEvent) error

	GetBranches(story *Story) ([]BranchRef, error)

}

