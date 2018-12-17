// +build windows

package bosun

import (
	"fmt"
	"strings"
)

func (e *EnvironmentConfig) render() string {
	w := new(strings.Builder)
	for _, v := range e.Variables {
		fmt.Fprintf(w, "SET %s=%s\n", v.Name, v.From.GetValue())
		fmt.Fprintf(w, "SETX %s=%s\n", v.Name, v.From.GetValue())
	}
	return w.String()
}


func executeScript(script string) (string, error) {
	tmp, err := ioutil.TempFile(os.TempDir(), "bosun-script")
	if err != nil {
		return "", err
	}
	tmp.Close()
	ioutil.WriteFile(tmp.Name(), []byte(script), 0700)

	defer os.Remove(tmp.Name())

	// pkg.Log.Debugf("running script from temp file %q", tmp.Name())
	cmd := exec.Command("cmd", tmp.Name())
	o, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(o), nil
}