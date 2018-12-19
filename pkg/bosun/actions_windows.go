// +build windows

package bosun

import (
	"fmt"
	"strings"
)

func  render(vars map[string]string) string {
	w := new(strings.Builder)

	for k, v := range vars {
		fmt.Fprintf(w, "SET %s=%s\n", k, v)
		fmt.Fprintf(w, "SETX %s=%s\n", k, v)
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