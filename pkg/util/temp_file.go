package util

import (
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
)

type TempFile struct {
	Content []byte
	Pattern string
	Path    string
	file    *os.File
}

func NewTempFile(pattern string, content []byte) (*TempFile, error) {
	tmp, err := ioutil.TempFile(os.TempDir(), pattern)
	if err != nil {
		return nil, err
	}
	_, err = tmp.Write(content)
	if err != nil {
		return nil, errors.Wrap(err, "write content")
	}

	return &TempFile{
		Content:content,
		Pattern:pattern,
		Path:tmp.Name(),
		file: tmp,
	}, nil
}

func (t *TempFile) CleanUp() error {
	if t.file != nil {
		err := t.file.Close()
		if err != nil {
			return err
		}
	}
	if t.Path != "" {
		err := os.Remove(t.Path)
		if err != nil {
			return err
		}
	}
	return nil
}
