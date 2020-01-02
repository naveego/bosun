package core

type ConfigShared struct {
	FromPath    string `yaml:"-" json:"fromPath"`
	Name        string `yaml:"name,omitempty" json:"name" json:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	File        Saver  `yaml:"-" json:"-"`
}

func (c *ConfigShared) SetFromPath(fp string) {
	c.FromPath = fp
}

func (c *ConfigShared) SetParent(p Saver) {
	c.File = p
}
