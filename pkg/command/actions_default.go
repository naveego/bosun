// +build !windows

package command

import (
	"fmt"
	"strings"
)

func RenderEnvironmentSettingScript(vars map[string]string, aliases map[string]string) string {
	w := new(strings.Builder)
	for k, v := range vars {
		fmt.Fprintf(w, "export %s=%s\n", k, v)
	}

	for k, v := range aliases {
		fmt.Fprintf(w, "alias %s=%q\n", k, v)
	}


	return w.String()
}

func GetCommandForScript(file string) *ShellExe {
	cmd := NewShellExe("/bin/bash", file)
	return cmd
}
