package kube

import (
	"encoding/base64"
	"fmt"
	"github.com/naveego/bosun/pkg/command"
	"github.com/pkg/errors"
	"io/ioutil"
	"strings"
	"text/template"
)

func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
			"kube_server": func(context string) (string, error) {
				if context == "" {
					return "", errors.New("context parameter was not set")
				}

				cluster, err := getClusterForContext(context)
				if err != nil {
					return "", err
				}

				o, err :=command.NewShellExe(fmt.Sprintf(`kubectl config view --raw -o jsonpath={.clusters[?(@.name=="%s")].cluster.server}`, cluster)).RunOut()
				return o, err
			},
			"kube_ca_cert": func(context string, optionalOverride ...string) (string, error) {

				if len(optionalOverride) > 0 && optionalOverride[0] != "" {
					return optionalOverride[0], nil
				}

				if context == "" {
					return "", errors.New("context parameter was not set")
				}

				cluster, err := getClusterForContext(context)
				if err != nil {
					return "", err
				}

				data, err :=command.NewShellExe(fmt.Sprintf(`kubectl config view --raw -o jsonpath={.clusters[?(@.name=="%s")].cluster.certificate-authority-data}`, cluster)).RunOut()

				if err != nil {
					return "", err
				}

				var cert []byte
				if data == "" {
					data, err =command.NewShellExe(fmt.Sprintf(`kubectl config view --raw -o jsonpath={.clusters[?(@.name=="%s")].cluster.certificate-authority}`, cluster)).RunOut()
					if err != nil {
						return "", err
					}
					cert, err = ioutil.ReadFile(data)
					if err != nil {
						return "", errors.Wrap(err, "get cert")
					}
				} else {
					cert, err = base64.StdEncoding.DecodeString(data)
					if err != nil {
						return "", errors.Errorf("could not decode %q: %s", data, err)
					}
				}

				return string(cert), nil
			},
			"kube_service_token": func(context string, serviceAccount string, optionalNamespace ...string) (string, error) {

				if context == "" {
					return "", errors.New("context parameter was not set")
				}

				namespace := "default"
				if len(optionalNamespace) > 0 {
					namespace = optionalNamespace[0]
				}

				o, err :=command.NewShellExe("kubectl", "--namespace", namespace, "--context", context, "get", "serviceaccounts", serviceAccount, "-o", "jsonpath={.secrets[0].name}").RunOut()
				if err != nil {
					return "", errors.Errorf("getting service account data for account %q in context %q: %s", serviceAccount, context, err)
				}

				o, err =command.NewShellExe(fmt.Sprintf(`kubectl --namespace %s  --context=%s get secrets %s -o jsonpath={.data.token}'`, namespace, context, o)).RunOut()
				if err != nil {
					return "", err
				}

				token := strings.Trim(o, "\"' ")
				var decoded []byte
				if !strings.Contains(token, ".") {
					decoded, err = base64.StdEncoding.DecodeString(token)
					token = string(decoded)
				}
				if err != nil {
					return "", errors.Errorf("decoding token %q: %s", o, err)
				}

				return string(token), nil
			},
		}

}



func getClusterForContext(context string) (string, error) {
	data, err :=command.NewShellExe(fmt.Sprintf(`kubectl config view --raw -o jsonpath={.contexts[?(@.name=="%s")].context.cluster}`, context)).RunOut()
	return data, err
}