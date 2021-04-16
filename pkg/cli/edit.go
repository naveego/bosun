package cli

import (
	"bytes"
	"fmt"
	"github.com/naveego/bosun/pkg/core"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
)

func Edit(targetPath string) error {
	editor, ok := os.LookupEnv("EDITOR")
	if !ok {
		return errors.New("EDITOR environment variable is not set")
	}

	currentBytes, err := ioutil.ReadFile(targetPath)
	if err != nil {
		return err
	}

	stat, err := os.Stat(targetPath)
	if err != nil {
		return errors.Wrap(err, "stat target file")
	}

	tmp, err := ioutil.TempFile(os.TempDir(), "bosun-*.yaml")
	if err != nil {
		return errors.Wrap(err, "temp file")
	}

	_, err = io.Copy(tmp, bytes.NewReader(currentBytes))
	if err != nil {
		return errors.Wrap(err, "copy to temp file")
	}
	err = tmp.Close()
	if err != nil {
		return errors.Wrap(err, "close temp file")
	}

	editorCmd := exec.Command("sh", "-c", fmt.Sprintf("%s %s", editor, tmp.Name()))

	editorCmd.Stderr = os.Stderr
	editorCmd.Stdout = os.Stdout
	editorCmd.Stdin = os.Stdin

	err = editorCmd.Run()
	if err != nil {
		return errors.Errorf("editor command %s failed: %s", editor, err)
	}

	updatedBytes, err := ioutil.ReadFile(tmp.Name())
	if err != nil {
		return errors.Wrap(err, "read updated file")
	}

	if bytes.Equal(currentBytes, updatedBytes) {
		core.Log.Info("No changes detected.")
		return nil
	}

	core.Log.WithField("path", targetPath).Info("Updating file.")

	err = ioutil.WriteFile(targetPath, updatedBytes, stat.Mode())
	if err != nil {
		return errors.Wrap(err, "write updated file")
	}

	return nil
}
