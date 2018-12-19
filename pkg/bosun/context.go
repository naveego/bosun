package bosun

import (
	"context"
	"os"
	"path/filepath"
)

type BosunContext struct {
	Bosun *Bosun
	Env *EnvironmentConfig
	Dir string
	Ctx context.Context
}

func (c BosunContext) ForDir(dir string) BosunContext {
	if stat, err := os.Stat(dir); err == nil {
		if !stat.IsDir() {
			dir = filepath.Dir(dir)
		}
	}
	c.Dir = dir
	return c
}