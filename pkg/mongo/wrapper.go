package mongo

import (
	"github.com/sirupsen/logrus"
	"gopkg.in/mgo.v2"
)

type mongoWrapper struct {
	session *mgo.Session
}

func (mw *mongoWrapper) DropCollection(name string) error {
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

func (mw *mongoWrapper) CreateCollection(col Collection) error {
	s := mw.session.Copy()
	defer s.Close()

	info := &mgo.CollectionInfo{
		Capped: safeBool(col.IsCapped),
	}

	if info.Capped {
		if col.MaxBytes != nil {
			info.MaxBytes = *col.MaxBytes
		}

		if col.MaxDocuments != nil {
			info.MaxDocs = *col.MaxDocuments
		}
	}

	return s.DB("").C(col.Name).Create(info)
}

func (mw *mongoWrapper) InsertData(colName string, data []map[string]interface{}) error {
	s := mw.session.Copy()
	defer s.Close()

	col := s.DB("").C(colName)

	for _, r := range data {
		err := col.Insert(r)
		if err != nil {
			return err
		}
	}
	return nil
}

func safeBool(b *bool) bool {
	if b == nil {
		return false
	}

	return *b
}
