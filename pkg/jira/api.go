package jira

type getBranchesResponse struct {
	Errors []interface{} `json:"errors"`
	Detail []struct {
		Branches []struct {
			Name                 string `json:"name"`
			URL                  string `json:"url"`
			CreatePullRequestUrl string `json:"createPullRequestUrl"`
			Repository           struct {
				Name     string        `json:"name"`
				URL      string        `json:"url"`
				Branches []interface{} `json:"branches"`
			} `json:"repository"`
			LastCommit struct {
				ID              string `json:"id"`
				Displayid       string `json:"displayId"`
				Authortimestamp string `json:"authorTimestamp"`
				URL             string `json:"url"`
				Author          struct {
					Name string `json:"name"`
				} `json:"author"`
				Filecount int           `json:"fileCount"`
				Merge     bool          `json:"merge"`
				Message   string        `json:"message"`
				Files     []interface{} `json:"files"`
			} `json:"lastCommit"`
		} `json:"branches"`
		PullRequests []interface{} `json:"pullRequests"`
		Repositories []interface{} `json:"repositories"`
		Instance     struct {
			Name           string `json:"name"`
			Baseurl        string `json:"baseUrl"`
			Type           string `json:"type"`
			ID             string `json:"id"`
			Typename       string `json:"typeName"`
			Singleinstance bool   `json:"singleInstance"`
		} `json:"_instance"`
	} `json:"detail"`
}

