package mongo

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"strings"
)

// ScriptCommand runs a database command: https://docs.mongodb.com/manual/reference/command/
type ScriptCommand struct {
	Conn   Connection
	Script string
	Log    *logrus.Entry `json:"-"`
}

func (m ScriptCommand) Execute() error {
	if m.Log == nil {
		m.Log = logrus.WithField("cmd", "MongoImportCommand")
	}

	pc, err := m.Conn.Prepare(m.Log)
	if err != nil {
		return err
	}
	if pc.CleanUp != nil {
		defer pc.CleanUp()
	}

	addr := fmt.Sprintf("mongodb://%s:%s@%s:%s/%s?authSource=%s",
		pc.Credentials.Username,
		pc.Credentials.Password,
		pc.Host,
		pc.Port,
		pc.DBName,
		pc.Credentials.AuthSource)

	safeAddr := strings.Replace(addr, pc.Credentials.Password, "%PASSWORD", 1)

	m.Log.Infof("Using connection string %s", safeAddr)
	m.Log.Infof("Executing script:\n %s", m.Script)

	err = pkg.NewCommand("mongo", addr, "--eval", m.Script).RunE()
	if err != nil {
		return errors.Wrapf(err, "running script %s", m.Script)
	}

	return nil
}
