package mongo

import (
	"fmt"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/mongoimport"
	"github.com/sirupsen/logrus"
	"gopkg.in/mgo.v2"
	"os"
	"path/filepath"
	"time"
)

type mongoWrapper struct {
	Host       string
	Port       string
	DBName     string
	Username   string
	Password   string
	AuthSource string
	DataDir    string

	session *mgo.Session
}

func newMongoWrapper(host, port, dbName, username, password, authSource, dataDir string) (*mongoWrapper, error) {
	w := &mongoWrapper{
		Host:       host,
		Port:       port,
		DBName:     dbName,
		Username:   username,
		Password:   password,
		AuthSource: authSource,
		DataDir:    dataDir,
	}

	hostAndPort := fmt.Sprintf("%s:%s", host, port)

	logrus.Infof("connecting to mongodb at %s", hostAndPort)
	logrus.Debugf("Username=%s", username)
	logrus.Debugf("AuthSource=%s", authSource)

	dialInfo := &mgo.DialInfo{
		Database: dbName,
		Addrs:    []string{hostAndPort},
		Direct:   true,
		Source:   authSource,
		Username: username,
		Password: password,
	}

	session, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		return nil, fmt.Errorf("could not connect to mongodb: %v", err)
	}

	w.session = session

	return w, nil
}

func (mw *mongoWrapper) Import(colName string, col CollectionInfo) error {

	if col.Drop {
		logrus.Infof("Dropping and re-creating collection '%s'", colName)
		err := mw.dropCollection(colName)
		if err != nil {
			return fmt.Errorf("error dropping collection '%s': %v", colName, err)
		}

		err = mw.createCollection(colName, col)
		if err != nil {
			return fmt.Errorf("error creating collection '%s': %v", colName, err)
		}
	}

	if col.Indexes != nil {
		for n, i := range col.Indexes {
			logrus.Debugf("ensuring index '%s' on collection '%s'", n, colName)
			err := mw.ensureIndex(colName, n, i)
			if err != nil {
				logrus.Warnf("could not ensure index '%s' on collection '%s': %v", n, colName, err)
			}
		}
	}

	if col.Data != nil && *col.Data != "" {
		err := mw.importData(colName, *col.Data)
		if err != nil {
			return fmt.Errorf("error inserting data into collection '%s': %v", colName, err)
		}
	}

	return nil
}

func (mw *mongoWrapper) importData(colName string, dataFile string) error {
	dataFilePath := filepath.Join(mw.DataDir, os.ExpandEnv(dataFile))

	logrus.Infof("importing data for collection '%s' from file '%s'", colName, dataFilePath)

	toolOptions := mw.getToolOptions(colName)
	inputOptions := &mongoimport.InputOptions{
		ParseGrace: "stop",
		File:       dataFilePath,
		Type:       "JSON",
	}
	ingestOptions := &mongoimport.IngestOptions{
		Mode: "upsert",
	}

	provider, err := db.NewSessionProvider(*toolOptions)
	if err != nil {
		return err
	}

	// Setup the MongoImport
	im := &mongoimport.MongoImport{
		ToolOptions:     toolOptions,
		InputOptions:    inputOptions,
		IngestOptions:   ingestOptions,
		SessionProvider: provider,
	}

	cnt, err := im.ImportDocuments()
	if err == nil {
		logrus.Infof("Successfully imported %d documents into %s.%s", cnt, mw.DBName, colName)
	}
	return err
}

func (mw *mongoWrapper) dropCollection(name string) error {
	s := mw.session.Copy()
	defer s.Close()

	names, err := s.DB("").CollectionNames()
	if err != nil {
		return err
	}

	for _, n := range names {
		if n == name {
			logrus.Debugf("Dropping collection '%s'", name)
			return s.DB("").C(name).DropCollection()
		}
	}

	return nil
}

func (mw *mongoWrapper) createCollection(colName string, col CollectionInfo) error {
	s := mw.session.Copy()
	defer s.Close()

	info := &mgo.CollectionInfo{
		Capped: col.IsCapped,
	}

	if info.Capped {
		if col.MaxBytes != nil {
			info.MaxBytes = *col.MaxBytes
		}

		if col.MaxDocuments != nil {
			info.MaxDocs = *col.MaxDocuments
		}
	}

	return s.DB("").C(colName).Create(info)
}

func (mw *mongoWrapper) ensureIndex(colName, indexName string, index IndexInfo) error {
	s := mw.session.Copy()
	defer s.Close()

	var keys []string
	for k, v := range index.Fields {
		if v == 1 {
			keys = append(keys, k)
		} else {
			keys = append(keys, "-"+k)
		}
	}

	i := mgo.Index{
		Name:   indexName,
		Sparse: index.Sparse,
		Unique: index.Unique,
		Key:    keys,
	}

	if index.ExpireAfter != nil {
		i.ExpireAfter = time.Duration(*index.ExpireAfter) * time.Second
	}

	return s.DB("").C(colName).EnsureIndex(i)
}

func (mw *mongoWrapper) getToolOptions(colName string) *options.ToolOptions {
	return &options.ToolOptions{
		General: &options.General{},
		SSL: &options.SSL{
			UseSSL: false,
		},
		Auth: &options.Auth{
			Username: mw.Username,
			Password: mw.Password,
			Source:   mw.AuthSource,
		},
		Namespace: &options.Namespace{
			DB:         mw.DBName,
			Collection: colName,
		},
		Connection: &options.Connection{
			Host: mw.Host,
			Port: mw.Port,
		},
		URI: &options.URI{},
	}

}
