package jira

import (
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/stories"
	"github.com/naveego/bosun/pkg/values"
	"regexp"
	"strings"
)

type Config struct {
	Provider     string               `yaml:"provider"`
	AccountID    string               `yaml:"accountID"`
	JiraUrl      string               `yaml:"url"`
	JiraUsername string               `yaml:"username"`
	JiraToken    command.CommandValue `yaml:"token"`
	IDPattern    string               `yaml:"idPattern"`
	Transitions  Transitions          `yaml:"transitions"`
}

type Transitions struct {
	InDevelopment string `yaml:"development,omitempty"`
	CodeReview    string `yaml:"codeReview,omitempty"`
	QA            string `yaml:"qa,omitempty"`
	UAT           string `yaml:"qa,omitempty"`
}

type CompiledTransitions struct {
	InDevelopment *regexp.Regexp
	CodeReview    *regexp.Regexp
	QA            *regexp.Regexp
	UAT           *regexp.Regexp
}

func (t Transitions) Compiled() (CompiledTransitions, error) {
	compiled := CompiledTransitions{}

	if t.InDevelopment == "" {
		t.InDevelopment = ".*development.*"
	}

	if t.CodeReview == "" {
		t.CodeReview = ".*code\\s+review.*"
	}
	if t.QA == "" {
		t.QA = ".*qa.*"
	}
	if t.UAT == "" {
		t.UAT = ".*uat.*"
	}

	var err error
	if compiled.InDevelopment, err = regexp.Compile("(?i)"+t.InDevelopment); err != nil {
		return compiled, err
	}

	if compiled.CodeReview, err = regexp.Compile("(?i)"+t.CodeReview); err != nil {
		return compiled, err
	}
	if compiled.QA, err = regexp.Compile("(?i)"+t.QA); err != nil {
		return compiled, err
	}
	if compiled.UAT, err = regexp.Compile("(?i)"+t.UAT); err != nil {
		return compiled, err
	}

	return compiled, nil
}

var Factory = stories.StoryRegistrationFactory{
	Factory: func(configs []values.Values) []stories.StoryHandlerRegistration {

		var results []stories.StoryHandlerRegistration
		for _, v := range configs {

			rawProvider, _ := v.GetAtPath("provider")
			provider, ok := rawProvider.(string)
			if ok && strings.ToLower(provider) == "jira" {

				var config Config
				err := v.Unmarshal(&config)
				if err != nil {
					core.Log.WithError(err).Error("Could not parse values.")
					continue
				}
				if config.IDPattern == "" {
					config.IDPattern = ".*"
				}

				pattern, reErr := regexp.Compile(config.IDPattern)
				if reErr != nil {
					core.Log.WithError(err).Error("Invalid ID pattern")
					continue
				}
				results = append(results, stories.StoryHandlerRegistration{
					IDPattern: pattern,
					HandlerFactory: func(ctx command.ExecutionContext) (stories.StoryHandler, error) {
						return NewClient(ctx, config)
					},
				})
			}
		}

		return results
	},
	Help: `Jira Story Handler:

Update your workspace bosun.yaml storyHandlers:

storyHandlers:
- provider: jira
  # A pattern that matches the IDs you want to handle
  idPattern: DATAINT-\d+  
  # URL to atlassian jira 
  url: https://aunalytics.atlassian.net
  # Your username
  username:
  # Your account ID; go to your profile in jira and copy the last path segment, like 557058:25a4ebee-6aa5-48c6-a186-561c36d52b33
  accountID: 
  # Obtain a token from https://id.atlassian.com/manage-profile/security/api-tokens and put it in LastPass
  token: 
    script:
      lpass show $FOLDER_AND_PATH --password
`,
}
