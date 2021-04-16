package command

import (
	"fmt"
	"github.com/naveego/bosun/pkg/templating/templatefuncs"
)

func init() {

	templatefuncs.Register("exec", func(exe string, args ...string) (string, error) {
		out, err := NewShellExe(exe, args...).RunOut()
		return out, err
	})

	templatefuncs.Register(
		"generateLastPassPassword", func(name, username, url string) (string, error) {
			password, err := NewShellExe("lpass", "show", "--sync=now", "-p", "--basic-regexp", name).RunOut()
			if err == nil {
				return password, nil
			}

			fmt.Printf("LPASS: password %q does not yet exist; it will be generated; %s\n", name, err)

			// password doesn't exist yet

			password, err = NewShellExe("lpass", "generate", "--sync=now", "--no-symbols", "--username", username, "--url", url, name, "30").RunOut()
			if err != nil {
				return "", err
			}

			return password, nil
		})
}
