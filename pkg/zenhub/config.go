package zenhub

import "github.com/naveego/bosun/pkg/issues"

type Config struct {
	StoryBoardName string `yaml:"storyBoardName"`
	TaskBoardName string `yaml:"taskBoardName"`
	TaskCompletedName string `yaml:"taskCompletedName"`
	GithubToken string `yaml:"-"`
	ZenhubToken string `yaml:"-"`
	StoryColumnMapping issues.ColumnMapping `yaml:"storyColumnMapping"`
	TaskColumnMapping issues.ColumnMapping `yaml:"taskColumnMapping"`
}

type PipelineColumnMapping struct {

}
