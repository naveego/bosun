// +build windows

package command

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"strings"
)

func RenderEnvironmentSettingScript(vars map[string]string) string {
	w := new(strings.Builder)

	for k, v := range vars {
		fmt.Fprintf(w, "SET %s=%s\n", k, v)
		fmt.Fprintf(w, "SETX %s=%s\n", k, v)
	}
	return w.String()
}

func GetCommandForScript(file string) *pkg.Command {
	cmd := pkg.NewCommand("cmd", "/q", "/c", file)
	return cmd
}