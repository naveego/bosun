package zenhub

type RepoConfig struct {
	ID int `yaml:"id" json:"id"`
}

// Repository represents a github repository.
type Repository struct {
	ID int `json:"id" yaml:"id"`
}

// User represents a github user.
type User struct {
	Login string `json:"login"`
	ID    int    `json:"id"`
}

// Assignees represents a list of logins associated with an issue.
type Assignees struct {
	List []string `json:"assignees"`
}

// Pipelines represents a slice of zenhub pipelines.
type Pipelines struct {
	List []Pipeline `json:"pipelines"`
}

// Pipeline represents a zenhub pipeline.
type Pipeline struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Issues      []Issue `json:"issues"`
	IssueNumber int     `json:"issue_number"`
	IsEpic      bool    `json:"is_epic"`
}

// Issue represents a zenhub issue.
type Issue struct {
	IssueNumber int      `json:"issue_number"`
	Estimate    Estimate `json:"estimate"`
	Position    int      `json:"position"`
	IsEpic      bool     `json:"is_epic"`
}

// Estimate represents a zenhub estimate.
type Estimate struct {
	Value int `json:"value"`
}

// PipelineMove represents the destination pipeline when moving an issue between pipelines.
type PipelineMove struct {
	PipelineID string `json:"pipeline_id"`
	Position   string `json:"position"`
}

type Dependency struct {
	Blocking DependencyIssue `json:"blocking"`
	Blocked DependencyIssue `json:"blocked"`
}

func NewDependency(blocking, blocked DependencyIssue) Dependency {
	d := Dependency{
		Blocking: blocking,
		Blocked: blocked,
	}
	return d
}

type DependencyIssue struct {
	RepoID      int `json:"repo_id"`
	IssueNumber int `json:"issue_number"`
}

func NewDependencyIssue(repoId, issueNumber int) DependencyIssue {

	di := DependencyIssue{
		RepoID: repoId,
		IssueNumber: issueNumber,
	}
	return di
}