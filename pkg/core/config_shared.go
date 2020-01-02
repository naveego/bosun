package core

import "github.com/naveego/bosun/pkg/bosun"

type ConfigShared struct {
	FromPath    string      `yaml:"-" json:"fromPath"`
	Name        string      `yaml:"name,omitempty" json:"name" json:"name" json:"name"`
	Description string      `yaml:"description,omitempty" json:"description,omitempty"`
	File        *bosun.File `yaml:"-" json:"-"`
}

func (c *ConfigShared) SetFromPath(fp string) {
	c.FromPath = fp
}

func (c *ConfigShared) SetParent(p *bosun.File) {
	c.File = p
}
