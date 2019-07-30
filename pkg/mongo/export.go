package mongo

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type MongoExportCommand struct {
	Conn    Connection
	DB      Database
	DataDir string
	Log     *logrus.Entry `json:"-"`
}

func (e MongoExportCommand) Execute() error {
	if e.Log == nil {
		e.Log = logrus.WithField("cmd", "MongoExportCommand")
	}

	if e.DB.Name == "" {
		e.DB.Name = e.Conn.DBName
	}
	if e.Conn.DBName == "" {
		e.Conn.DBName = e.DB.Name
	}

	pc, err := e.Conn.Prepare(e.Log)
	if err != nil {
		return errors.Wrap(err, "prepare connection")
	}

	wrapper, err := e.getMongoWrapper(e.DataDir, pc)
	if err != nil {
		return errors.Wrap(err, "connect to mongo")
	}
	if pc.CleanUp != nil {
		defer pc.CleanUp()
	}

	for name, info := range e.DB.Collections {
		err = wrapper.exportData(name, info.DataFile)
		if err != nil {
			logrus.Warnf("could not export collection %q from %q to %q: %v", name, e.DB.Name, info.DataFile, err)
		}
	}

	return nil
}

func (e MongoExportCommand) getMongoWrapper(dataDir string, c PreparedConnection) (*mongoWrapper, error) {
	wrapper, err := newMongoWrapper(
		c.Host,
		c.Port,
		c.DBName,
		c.Credentials.Username,
		c.Credentials.Password,
		c.Credentials.AuthSource,
		dataDir,
		e.Log.WithField("typ", "mongoWrapper"))

	if err != nil {
		return nil, fmt.Errorf("could not get mongo wrapper: %v", err)
	}

	return wrapper, nil
}
