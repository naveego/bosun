package bosun

import (
	"context"
	vault "github.com/hashicorp/vault/api"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
)

type BosunContext struct {
	Bosun *Bosun
	Env *EnvironmentConfig
	Dir string
	Log *logrus.Entry
	Ctx context.Context
}




func (c BosunContext) WithDir(dir string) BosunContext {
	if stat, err := os.Stat(dir); err == nil {
		if !stat.IsDir() {
			dir = filepath.Dir(dir)
		}
	}
	c.Dir = dir
	return c
}

func (c BosunContext) WithLog(log *logrus.Entry) BosunContext {
	c.Log = log
	return c
}

func (c BosunContext) GetVaultClient() (*vault.Client, error){
	return c.Bosun.GetVaultClient()
}

func (c BosunContext) IsDryRun() bool {
	return c.Bosun.params.DryRun
}