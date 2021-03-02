package stories

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/values"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"regexp"
)

type StoryHandlerRegistration struct {
	IDPattern      *regexp.Regexp
	HandlerFactory func(ctx command.ExecutionContext) (StoryHandler, error)
}

type registeredStoryHandler struct {
	IDPattern           *regexp.Regexp
	HandlerFactory      func(ctx command.ExecutionContext) (StoryHandler, error)
	RegistrationFactory StoryRegistrationFactory
	handler             StoryHandler
}

type StoryRegistrationFactory struct {
	Factory func(v []values.Values) []StoryHandlerRegistration
	Help    string
}

func (r *registeredStoryHandler) GetHandler(ctx command.ExecutionContext) (StoryHandler, error) {
	var err error
	if r.handler == nil {
		r.handler, err = r.HandlerFactory(ctx)
	}
	return r.handler, err
}

var DefaultStoryRegister = &StoryRegister{}

func RegisterFactory(factory StoryRegistrationFactory) {
	DefaultStoryRegister.RegisterFactory(factory)
}

func Configure(storyHandlerConfigs []values.Values) {
	DefaultStoryRegister.Configure(storyHandlerConfigs)
}

func GetStoryHandler(ctx command.ExecutionContext, storyID string) (StoryHandler, error) {
	return DefaultStoryRegister.GetStoryClient(ctx, storyID)
}

type StoryRegister struct {
	registeredFactories []StoryRegistrationFactory
	registrations       []registeredStoryHandler
	configuration       []values.Values
}

func (s *StoryRegister) RegisterFactory(factory StoryRegistrationFactory) {

	s.registeredFactories = append(s.registeredFactories, factory)
}

func (s *StoryRegister) Configure(storyHandlerConfigs []values.Values) {
	if s.registrations == nil {
		s.configuration = storyHandlerConfigs
		for _, fn := range s.registeredFactories {
			for _, shr := range fn.Factory(storyHandlerConfigs) {

				rsh := registeredStoryHandler{
					IDPattern:           shr.IDPattern,
					HandlerFactory:      shr.HandlerFactory,
					RegistrationFactory: fn,
					handler:             nil,
				}
				s.registrations = append(s.registrations, rsh)
			}

		}
	}
}

func (s *StoryRegister) GetStoryClient(ctx command.ExecutionContext, storyID string) (StoryHandler, error) {

	for _, r := range s.registrations {
		if r.IDPattern.MatchString(storyID) {
			h, err := r.GetHandler(ctx)
			return h, err
		}
	}

	if len(s.registeredFactories) == 0 {
		return nil, errors.Errorf("no story handlers are registered, that's pretty strange")
	}

	color.Red("No story handlers found for story ID %q", storyID)

	color.Blue("Registered story handlers help info:\n")

	for _, f := range s.registeredFactories {
		fmt.Println(f.Help)
	}

	fmt.Println("----------------------------------")
	color.Blue("Current story handlers configuration:\n")
	configs, _ := yaml.MarshalString(s.configuration)
	fmt.Println(configs)

	fmt.Println()
	color.Blue("Current story handlers patterns:\n")
	for _, f := range s.registrations {
		fmt.Println(f.IDPattern)
	}

	return nil, errors.Errorf("no story handlers found for story ID %q", storyID)

}
