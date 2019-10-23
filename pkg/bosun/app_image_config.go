package bosun

import "fmt"

type AppImageConfig struct {
	ImageName    string   `yaml:"imageName" json:"imageName,omitempty"`
	ProjectName  string   `yaml:"projectName,omitempty" json:"projectName,omitempty"`
	Dockerfile   string   `yaml:"dockerfile,omitempty" json:"dockerfile,omitempty"`
	ContextPath  string   `yaml:"contextPath,omitempty" json:"contextPath,omitempty"`
	BuildCommand []string `yaml:"buildCommand,omitempty" json:"buildCommand,omitempty"`
}

func (a AppImageConfig) GetFullName() string {
	return fmt.Sprintf("docker.n5o.black/%s/%s", a.ProjectName, a.ImageName)
}

func (a AppImageConfig) GetFullNameWithTag(tag string) string {
	return fmt.Sprintf("docker.n5o.black/%s/%s:%s", a.ProjectName, a.ImageName, tag)
}

func (a *AppImageConfig) MarshalYAML() (interface{}, error) {
	if a == nil {
		return nil, nil
	}
	type proxy AppImageConfig
	p := proxy(*a)

	return &p, nil
}

func (a *AppImageConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type proxy AppImageConfig
	var p proxy
	if a != nil {
		p = proxy(*a)
	}

	err := unmarshal(&p)
	if err != nil {
		return err
	}

	*a = AppImageConfig(p)

	// handle "name" as "imageName"
	var m map[string]string
	_ = unmarshal(&m)
	if name, ok := m["name"]; ok {
		a.ImageName = name
	}

	return err
}
