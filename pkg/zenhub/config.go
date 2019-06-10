package zenhub

type Config struct {
	StoryBoardName string `yaml:"storyBoardName"`
	TaskBoardName string `yaml:"taskBoardName"`
	TaskCompletedName string `yaml:"taskCompletedName"`
	GithubToken string `yaml:"-"`
	ZenhubToken string `yaml:"-"`
}
