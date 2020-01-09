package core

import "path/filepath"

type ConfigShared struct {
	FromPath    string    `yaml:"-" json:"fromPath"`
	Name        string    `yaml:"name,omitempty" json:"name"`
	Description string    `yaml:"description,omitempty" json:"description,omitempty"`
	FileSaver   FileSaver `yaml:"-" json:"-"`
}

func (c *ConfigShared) SetFromPath(fp string) {
	c.FromPath = fp
}

func (c *ConfigShared) SetFileSaver(p FileSaver) {
	c.FileSaver = p
}

func (c *ConfigShared) ResolveRelative(path string) string {
	if c.FromPath == "" {
		panic("FromPath was not set (SetFromPath should have been called after loading)")
	}
	return filepath.Join(filepath.Dir(c.FromPath), path)
}

type FileSaverSetter interface {
	SetFileSaver(p FileSaver)
}

type FromPathSetter interface {
	SetFromPath(string)
}

var _ FromPathSetter = &ConfigShared{}
var _ FileSaverSetter = &ConfigShared{}
