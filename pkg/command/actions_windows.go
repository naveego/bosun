// +build windows

package command

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"strings"
)

func RenderEnvironmentSettingScript(vars map[string]string, aliases map[string]string) string {
	w := new(strings.Builder)

	for k, v := range vars {
		fmt.Fprintf(w, "SET %s=%s\n", k, v)
		fmt.Fprintf(w, "SETX %s=%s\n", k, v)
	}

	for k, v := range aliases {
		fmt.Fprintf(w, "doskey %s=%s\n", k, v)
	}

	return w.String()
}

func GetCommandForScript(file string) *pkg.ShellExe {
	cmd := pkg.NewShellExe("cmd", "/q", "/c", file)
	return cmd
}
