package mongo

import (
	"fmt"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/progress"
	"github.com/mongodb/mongo-tools/mongoexport"
	"github.com/mongodb/mongo-tools/mongoimport"
	"github.com/pkg/errors"
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
	log     *logrus.Entry
}

func newMongoWrapper(host, port, dbName, username, password, authSource, dataDir string, log *logrus.Entry) (*mongoWrapper, error) {
	w := &mongoWrapper{
		Host:       host,
		Port:       port,
		DBName:     dbName,
		Username:   username,
		Password:   password,
		AuthSource: authSource,
		DataDir:    dataDir,
		log:        log,
	}
	if w.log == nil {
		w.log = logrus.WithField("typ", "mongoWrapper")
	}

	hostAndPort := fmt.Sprintf("%s:%s", host, port)

	w.log.Infof("connecting to mongodb at %s", hostAndPort)
	w.log.Debugf("Username=%s", username)
	w.log.Debugf("AuthSource=%s", authSource)

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
		return nil, errors.Wrap(err, "dial mongo")
	}

	w.session = session

	return w, nil
}

func (w *mongoWrapper) Import(colName string, col CollectionInfo) error {

	if col.Drop {
		w.log.Infof("Dropping and re-creating collection '%s'", colName)
		err := w.dropCollection(colName)
		if err != nil {
			return fmt.Errorf("error dropping collection '%s': %v", colName, err)
		}

		err = w.createCollection(colName, col)
		if err != nil {
			return fmt.Errorf("error creating collection '%s': %v", colName, err)
		}
	}

	if col.Indexes != nil {
		for n, i := range col.Indexes {
			w.log.Debugf("ensuring index '%s' on collection '%s'", n, colName)
			err := w.ensureIndex(colName, n, i)
			if err != nil {
				logrus.Warnf("could not ensure index '%s' on collection '%s': %v", n, colName, err)
			}
		}
	}

	if col.DataFile != "" {
		err := w.importData(colName, col.DataFile)
		if err != nil {
			return fmt.Errorf("error inserting data into collection '%s': %v", colName, err)
		}
	}

	return nil
}

func (w *mongoWrapper) importData(colName string, dataFile string) error {

	dataFilePath := os.ExpandEnv(dataFile)
	if !filepath.IsAbs(dataFilePath) {
		dataFilePath = filepath.Join(w.DataDir, dataFilePath)
	}

	w.log.Infof("importing data for collection '%s' from file '%s'", colName, dataFilePath)

	toolOptions := w.getToolOptions(colName)
	inputOptions := &mongoimport.InputOptions{
		ParseGrace: "stop",
		File:       dataFilePath,
		Type:       "JSON",
	}
	ingestOptions := &mongoimport.IngestOptions{
		Mode:         "upsert",
		UpsertFields: "_id",
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

	err = im.ValidateSettings(nil)
	if err != nil {
		return errors.Wrap(err, "validate mongo import settings")
	}

	// log.SetVerbosity(options.Verbosity{Quiet:false, VLevel:log.DebugLow})

	cnt, err := im.ImportDocuments()
	if err == nil {
		w.log.Infof("Successfully imported %d documents into %s.%s", cnt, w.DBName, colName)
	}
	return err
}

func (w *mongoWrapper) exportData(colName string, dataFile string) error {
	dataFilePath := os.ExpandEnv(dataFile)
	if !filepath.IsAbs(dataFilePath) {
		dataFilePath = filepath.Join(w.DataDir, dataFilePath)
	}

	w.log.Infof("exporting data for collection '%s' to file '%s'", colName, dataFilePath)

	toolOptions := w.getToolOptions(colName)
	exportOptions := &mongoexport.InputOptions{
		AssertExists: true,
	}

	provider, err := db.NewSessionProvider(*toolOptions)
	if err != nil {
		return err
	}

	output, err := os.OpenFile(dataFilePath, os.O_TRUNC|os.O_CREATE|os.O_RDWR, 0660)
	if err != nil {
		return err
	}

	defer output.Close()

	// Setup the MongoImport
	me := &mongoexport.MongoExport{
		ToolOptions: *toolOptions,
		InputOpts:   exportOptions,
		OutputOpts: &mongoexport.OutputFormatOptions{
			JSONArray: false,
			Type:      "json",
		},
		SessionProvider: provider,
		ProgressManager: progress.NewBarWriter(os.Stderr, 1*time.Second, 20, false),
	}

	err = me.ValidateSettings()
	if err != nil {
		return errors.Wrap(err, "validate mongo import settings")
	}

	// log.SetVerbosity(options.Verbosity{Quiet:false, VLevel:log.DebugLow})

	cnt, err := me.Export(output)
	if err == nil {
		w.log.Infof("Successfully exported %d documents into %q", cnt, dataFilePath)
	}
	return err
}

func (w *mongoWrapper) dropCollection(name string) error {
	s := w.session.Copy()
	defer s.Close()

	names, err := s.DB("").CollectionNames()
	if err != nil {
		return err
	}

	for _, n := range names {
		if n == name {
			w.log.Debugf("Dropping collection '%s'", name)
			return s.DB("").C(name).DropCollection()
		}
	}

	return nil
}

func (w *mongoWrapper) createCollection(colName string, col CollectionInfo) error {
	s := w.session.Copy()
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

func (w *mongoWrapper) ensureIndex(colName, indexName string, index IndexInfo) error {
	s := w.session.Copy()
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

func (w *mongoWrapper) getToolOptions(colName string) *options.ToolOptions {
	return &options.ToolOptions{
		General: &options.General{},
		SSL: &options.SSL{
			UseSSL: false,
		},
		Auth: &options.Auth{
			Username: w.Username,
			Password: w.Password,
			Source:   w.AuthSource,
		},
		Namespace: &options.Namespace{
			DB:         w.DBName,
			Collection: colName,
		},
		Connection: &options.Connection{
			Host: w.Host,
			Port: w.Port,
		},
		URI: &options.URI{},
	}

}
