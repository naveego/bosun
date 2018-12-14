// +build windows

package bosun

import (
	"fmt"
	"strings"
)

func (e *EnvironmentConfig) render() string {
	w := new(strings.Builder)
	for _, v := range e.Variables {
		fmt.Fprintf(w, "SET %s=%s\n", v.Name, v.Value)
		fmt.Fprintf(w, "SETX %s=%s\n", v.Name, v.Value)
	}
	return w.String()
}

func (e *EnvironmentVariable) ensureWithScript() (string, error) {
	tmp, err := ioutil.TempFile(os.TempDir(), "bosun-env")
	if err != nil {
		return "", err
	}
	defer func(){
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	cmd := exec.Command("cmd", tmp.Name())
	o, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(o), nil
}
