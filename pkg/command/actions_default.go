// +build !windows

package command

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"strings"
)

func RenderEnvironmentSettingScript(vars map[string]string) string {
	w := new(strings.Builder)
	for k, v := range vars {
		fmt.Fprintf(w, "export %s=%s\n", k, v)
	}
	return w.String()
}

func GetCommandForScript(file string) *pkg.ShellExe {
	cmd := pkg.NewShellExe("/bin/bash", file)
	return cmd
}
