package zenhub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/google/go-github/v20/github"
	"github.com/pkg/errors"
	//"io"
	"io/ioutil"
	"net/http"
	"strings"
)

const (
	zenhubRoot = "https://api.zenhub.io"
)

// API provides methods for interacting with ZenHub.
type API struct {
	githubAPI       *github.Client
	zenHubAuthToken string
}




// New returns a reference to a ZenHub API
func New(zenHubAuthToken string, githubAPI *github.Client) *API {
	return &API{
		zenHubAuthToken: zenHubAuthToken,
		githubAPI:       githubAPI,
	}
}

// GetZenHubWorkspaces returns all workspaces containing repo_id
func (a *API) GetWorkspaces(repoID int) (*Workspaces, error) {

	client := http.DefaultClient
	getWorkspacesURI := fmt.Sprintf("%v/p2/repositories/%v/workspaces", zenhubRoot, repoID)
	request, err := a.createDefaultRequest(http.MethodGet, getWorkspacesURI)
	if err != nil {
		return nil, err
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the get workspaces endpoint returned %v", response.StatusCode)
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	workspaces := new(Workspaces)
	err = json.Unmarshal(body, workspaces)

	return workspaces, err
}

func (a *API) GetWorkspaceID(repoID int, workspaceName string) (string, error) {
	workspaceID := ""
	workspaces, err := a.GetWorkspaces(repoID)
	if err != nil {
		return "", errors.Wrap(err, "get workspaces")
	}
	for _, workspace := range workspaces.List {
		if strings.ToLower(workspace.Name) == strings.ToLower(workspaceName) {
			workspaceID = workspace.ID
			break
		}
	}

	if workspaceID == "" {
		return "", fmt.Errorf("workspace '%v' does not exist for this board", workspaceName)
	}

	return workspaceID, nil
}

// GetPipelines returns a list of pipelines.
// This function uses the zenhub API "Get a ZenHub Board for a repo"
func (a *API) GetPipelines(workspaceID string, repoID int) (*Pipelines, error) {
	client := http.DefaultClient
	getPipelinesURI := fmt.Sprintf("%v/p2/workspaces/%v/repositories/%v/board", zenhubRoot, workspaceID, repoID)
	request, err := a.createDefaultRequest(http.MethodGet, getPipelinesURI)
	if err != nil {
		return nil, err
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the get pipelines endpoint returned %v", response.StatusCode)
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	pipelines := new(Pipelines)
	err = json.Unmarshal(body, pipelines)

	return pipelines, err
}

// MovePipeline moves the specified issue to the specified pipeline.
func (a *API) MovePipeline(workspaceID string, repoID int, issue int, pipelineID string) error {
	pipelineMove := &PipelineMove{
		PipelineID: pipelineID,
		Position:   "top",
	}
	pipelineMoveJSON, err := json.Marshal(pipelineMove)
	if err != nil {
		return errors.Wrap(err, "marshal move pipeline json")
	}

	client := http.DefaultClient
	getPipelinesURI := fmt.Sprintf("%v/p2/workspaces/%v/repositories/%v/issues/%v/moves", zenhubRoot, workspaceID, repoID, issue)
	request, err := a.createDefaultRequest(http.MethodPost, getPipelinesURI)
	if err != nil {
		return err
	}

	request.Header.Add("Content-Type", "application/json")
	request.Body = ioutil.NopCloser(bytes.NewReader(pipelineMoveJSON))
	response, err := client.Do(request)

	if err != nil {
		return errors.New("cannot move issue")
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("the move issue endpoint returned %v", response.StatusCode)
	}

	dumpJSON("status code ", response.StatusCode)
	return nil
}

func (a *API) AddDependency(dependency *Dependency) error {

	dependencyJSON, err := json.Marshal(dependency)
	if err != nil {
		return err
	}

	client := http.DefaultClient
	getPipelinesURI := fmt.Sprintf("%v/p1/dependencies", zenhubRoot)
	request, err := a.createDefaultRequest(http.MethodPost, getPipelinesURI)
	if err != nil {
		return err
	}

	request.Header.Add("Content-Type", "application/json")
	request.Body = ioutil.NopCloser(bytes.NewReader(dependencyJSON))
	response, err := client.Do(request)

	if err != nil {
		return err
	}
	if response.Body != nil {
		err = response.Body.Close()
		if err != nil {
			return errors.Wrap(err, "close response body")
		}
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("the dependency issue endpoint returned %v", response.StatusCode)
	}
	return nil
}

// GetParents returns dependencies of the specified issue in the repo.
func (a *API) GetDependencies(repoID, issueNum int) ([]Dependency, error) {

	var parents []int
	var children []int
	var depResponse DependenciesPackage
	client := http.DefaultClient

	getDependenciesURI := fmt.Sprintf("%v/p1/repositories/%v/dependencies", zenhubRoot, repoID)
	request, err := a.createDefaultRequest(http.MethodGet, getDependenciesURI)
	if err != nil {
		return nil,  errors.Wrap(err, "create zenhub request")
	}

	response, err := client.Do(request)
	if err != nil {
		return nil,  err
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the get dependencies endpoint returned %v", response.StatusCode)
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(body, &depResponse)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal json")
	}
	//dumpJSON("depResponse", depResponse)

	i := 0
	for ; i < len(depResponse.Dependencies); i++ {
		if depResponse.Dependencies[i].Blocking.IssueNumber == issueNum {
			parents = append(parents, depResponse.Dependencies[i].Blocked.IssueNumber)
		} else if depResponse.Dependencies[i].Blocked.IssueNumber == issueNum {
			children = append(children, depResponse.Dependencies[i].Blocking.IssueNumber)
		}
	}

	return depResponse.Dependencies, nil
}

func (a *API) GetIssueData(repoID, issueNumber int) (*IssueData, error) {

	client := http.DefaultClient
	// Use the Get Issue Data api
	getCurrentPipelineURI := fmt.Sprintf("%v/p1/repositories/%v/issues/%v", zenhubRoot, repoID, issueNumber)
	request, err := a.createDefaultRequest(http.MethodGet, getCurrentPipelineURI)
	if err != nil {
		return nil, errors.New("create default request")
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the get issue data endpoint returned %v", response.StatusCode)
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	issue := new(IssueData)
	err = json.Unmarshal(body, issue)
	if err != nil {
		return nil, errors.Wrap(err, "get issue data")
	}

	return issue, nil
}

// GetPipelineID returns the ZenHub ID for the specified pipeline name. If the specified pipeline
// does not exist for the current board, this method will return an empty string and an error.
func (a *API) GetPipelineID(workspaceID string, repoID int, pipelineName string) (string, error) {
	pipelineID := ""
	pipelines, err := a.GetPipelines(workspaceID, repoID)
	if err != nil {
		return "", errors.Wrap(err, "get pipelines")
	}
	for _, pipeline := range pipelines.List {
		if strings.ToLower(pipeline.Name) == strings.ToLower(pipelineName) {
			pipelineID = pipeline.ID
			break
		}
	}

	if pipelineID == "" {
		return "", fmt.Errorf("pipeline '%v' does not exist for this board", pipelineName)
	}

	return pipelineID, nil
}

func (a *API) createDefaultRequest(method, uri string) (*http.Request, error) {
	request, err := http.NewRequest(method, uri, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Add("X-Authentication-Token", a.zenHubAuthToken)
	return request, nil
}
