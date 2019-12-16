package pkg

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Masterminds/sprig"
	"github.com/fatih/color"
	vault "github.com/hashicorp/vault/api"
	"github.com/imdario/mergo"
	"github.com/pkg/errors"
	"github.com/rs/xid"
	"golang.org/x/crypto/ssh/terminal"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type TemplateHelper struct {
	TemplateValues TemplateValues
	VaultClient    *vault.Client
}

func (h *TemplateHelper) LoadFromYaml(out interface{}, globs ...string) error {

	mergedYaml, err := h.LoadMergedYaml(globs...)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal([]byte(mergedYaml), out)

	return err
}

func (h *TemplateHelper) LoadMergedYaml(globs ...string) (string, error) {

	var merged map[string]interface{}

	var paths []string
	for _, glob := range globs {
		p, err := filepath.Glob(glob)

		if err != nil {
			return "", err
		}
		paths = append(paths, p...)
	}

	if len(paths) == 0 {
		return "", errors.Errorf("no paths found from expanding %v", globs)
	}

	for _, path := range paths {

		b, err := ioutil.ReadFile(path)
		if err != nil {
			return "", err
		}

		builder := NewTemplateBuilder(path).
			WithKubeFunctions().
			WithTemplate(string(b))
		if h.VaultClient != nil {
			builder = builder.WithVaultTemplateFunctions(h.VaultClient)
		} else {
			builder = builder.WithDisabledVaultTemplateFunctions()
		}

		yamlString, err := builder.BuildAndExecute(h.TemplateValues)

		if err != nil {
			return "", err
		}

		var current map[string]interface{}

		err = yaml.Unmarshal([]byte(yamlString), &current)
		if err != nil {
			var badLine int
			matches := lineExtractor.FindStringSubmatch(err.Error())
			if len(matches) > 0 {
				badLine, _ = strconv.Atoi(matches[1])
			}
			color.Red("Invalid yaml in %s at line %d:", path, badLine)

			//fmt.Println(yamlString)

			return "", err
		}

		if err = mergo.Merge(&merged, current); err != nil {
			return "", err
		}
	}

	mergedYaml, err := yaml.Marshal(merged)

	return string(mergedYaml), err
}

type TemplateBuilder struct {
	t       *template.Template
	docs    []TemplateFuncDocs
	content string
}

type TemplateFuncDocs struct {
	Name        string
	Description string
	Args        []string
}

func NewTemplateBuilder(name string) *TemplateBuilder {
	t := template.New(name).
		Funcs(sprig.TxtFuncMap()).
		Funcs(template.FuncMap{
			"exec": func(exe string, args ...string) (string, error) {
				out, err := NewCommand(exe, args...).RunOut()
				return out, err
			},
			"xid": func() string {
				return xid.New().String()
			},
			"generateLastPassPassword": func(name, username, url string) (string, error) {
				password, err := NewCommand("lpass", "show", "--sync=now", "-p", "--basic-regexp", name).RunOut()
				if err == nil {
					return password, nil
				}

				fmt.Printf("LPASS: password %q does not yet exist; it will be generated; %s\n", name, err)

				// password doesn't exist yet

				password, err = NewCommand("lpass", "generate", "--sync=now", "--no-symbols", "--username", username, "--url", url, name, "30").RunOut()
				if err != nil {
					return "", err
				}

				return password, nil
			},
		})

	return &TemplateBuilder{t: t}
}

func (t *TemplateBuilder) Build() (*template.Template, error) {
	var err error
	t.t, err = t.t.Parse(t.content)
	return t.t, errors.WithStack(err)
}

func (t *TemplateBuilder) BuildAndExecute(input interface{}) (string, error) {

	o, err := t.Build()
	if err != nil {
		return "", err
	}

	w := new(strings.Builder)
	err = o.Execute(w, input)
	if err != nil {
		return "", errors.WithStack(err)
	}

	return w.String(), nil
}

func (t *TemplateBuilder) WithTemplate(c string) *TemplateBuilder {
	t.content = c
	return t
}

func (t *TemplateBuilder) AddFunc(name, description string, args []string, fn interface{}) {
	t.docs = append(t.docs, TemplateFuncDocs{
		Name:        name,
		Description: description,
		Args:        args,
	})

	t.t.Funcs(template.FuncMap{
		name: fn,
	})
}

func (t *TemplateBuilder) WithKubeFunctions() *TemplateBuilder {

	t.t = t.t.Funcs(template.FuncMap{
		"kube_server": func(context string) (string, error) {
			if context == "" {
				return "", errors.New("context parameter was not set")
			}

			cluster, err := getClusterForContext(context)
			if err != nil {
				return "", err
			}

			o, err := NewCommand(fmt.Sprintf(`kubectl config view --raw -o jsonpath={.clusters[?(@.name=="%s")].cluster.server}`, cluster)).RunOut()
			return o, err
		},
		"kube_ca_cert": func(context string) (string, error) {
			if context == "" {
				return "", errors.New("context parameter was not set")
			}

			cluster, err := getClusterForContext(context)
			if err != nil {
				return "", err
			}

			data, err := NewCommand(fmt.Sprintf(`kubectl config view --raw -o jsonpath={.clusters[?(@.name=="%s")].cluster.certificate-authority-data}`, cluster)).RunOut()

			if err != nil {
				return "", err
			}

			var cert []byte
			if data == "" {
				data, err := NewCommand(fmt.Sprintf(`kubectl config view --raw -o jsonpath={.clusters[?(@.name=="%s")].cluster.certificate-authority}`, cluster)).RunOut()
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

			o, err := NewCommand("kubectl", "--namespace", namespace, "--context", context, "get", "serviceaccounts", serviceAccount, "-o", "jsonpath={.secrets[0].name}").RunOut()
			if err != nil {
				return "", errors.Errorf("getting service account data for account %q in context %q: %s", serviceAccount, context, err)
			}

			o, err = NewCommand(fmt.Sprintf(`kubectl --namespace %s  --context=%s get secrets %s -o jsonpath={.data.token}'`, namespace, context, o)).RunOut()
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
	})

	return t

}

func getClusterForContext(context string) (string, error) {
	data, err := NewCommand(fmt.Sprintf(`kubectl config view --raw -o jsonpath={.contexts[?(@.name=="%s")].context.cluster}`, context)).RunOut()
	return data, err
}

func (t *TemplateBuilder) WithDisabledVaultTemplateFunctions() *TemplateBuilder {

	t.t = t.t.Funcs(template.FuncMap{
		"vaultWrappedAppRoleToken": func(role string) (string, error) {
			return "disabled", nil
		},

		"vaultTokenWithPolicy": func(policy string) (string, error) {
			return "disabled", nil
		},

		"vaultTokenWithRole": func(role string) (string, error) {
			return "disabled", nil
		},

		"vaultSecret": func(path string, optionalKey ...string) (string, error) {
			return "disabled", nil
		},
	})

	return t
}

func (t *TemplateBuilder) WithVaultTemplateFunctions(client *vault.Client) *TemplateBuilder {
	t.t = t.t.Funcs(template.FuncMap{
		"vaultWrappedAppRoleToken": func(role string) (string, error) {
			return CreateWrappedAppRoleToken(client, role)
		},

		"vaultTokenWithPolicy": func(policy string) (string, error) {
			req := &vault.TokenCreateRequest{
				Renewable:   to.BoolPtr(false),
				TTL:         "1m",
				NumUses:     1,
				Policies:    []string{policy},
				DisplayName: "deploy-token",
			}
			secret, err := client.Auth().Token().Create(req)
			if err != nil {
				return "", err
			}
			return secret.TokenID()
		},

		"vaultTokenWithRole": func(role string) (string, error) {
			req := &vault.TokenCreateRequest{
				DisplayName: "deploy-token",
			}
			secret, err := client.Auth().Token().CreateWithRole(req, role)
			if err != nil {
				return "", err
			}
			return secret.TokenID()
		},

		"vaultSecret": func(path string, optionalKeyAndDefault ...string) (string, error) {
			key := ""
			defaultValue := ""
			switch len(optionalKeyAndDefault) {
			case 1:
				key = optionalKeyAndDefault[0]
			case 2:
				key = optionalKeyAndDefault[0]
				defaultValue = optionalKeyAndDefault[1]
			}

			action := GetOrUpdateVaultSecretAction{
				Path:         path,
				Key:          key,
				DefaultValue: defaultValue,
				Client:       client,
			}

			return action.Execute()
		},
		"jwt": func(role string, ttl string, claimPairs ...string) (string, error) {

			claims := map[string]interface{}{}
			for i := 0; i < len(claimPairs)-1; i += 2 {
				claims[claimPairs[i]] = claimPairs[i+1]
			}

			ttlDuration, err := time.ParseDuration(ttl)
			if err != nil {
				return "", errors.Wrapf(err, "invalid ttl: wanted go duration, got %q", ttl)
			}

			exp := time.Now().Add(ttlDuration).Unix()
			claims["exp"] = exp

			req := map[string]interface{}{
				"claims":    claims,
				"token_ttl": ttlDuration.Seconds(),
			}

			s, err := client.Logical().Write(fmt.Sprintf("jose/jwt/issue/%s", role), req)
			if err != nil {
				return "", errors.Wrap(err, "post request")
			}

			token, ok := s.Data["token"].(string)
			if !ok {
				j, _ := json.Marshal(s)
				return "", errors.Errorf("secret did not contain token, got %s", string(j))
			}

			return token, nil
		},
	})

	return t
}

type GetOrUpdateVaultSecretAction struct {
	Path         string
	Key          string
	DefaultValue string
	Replace      bool
	Client       *vault.Client
}

func (g GetOrUpdateVaultSecretAction) Execute() (string, error) {

	var secretValue string
	client := g.Client
	path := g.Path
	key := g.Key
	defaultValue := g.DefaultValue

	update := g.Replace

	secret, err := client.Logical().Read(path)
	if err != nil {
		return "", err
	}

	if secret != nil && secret.Data != nil {

		if key == "" && len(secret.Data) == 1 {
			// user didn't specify Key, but there is only one
			// value, so we'll use that
			for _, v := range secret.Data {
				secretValue, _ = v.(string)
				break
			}
		} else {
			value, ok := secret.Data[key]
			if ok {
				// secret contained requested Key
				secretValue, _ = value.(string)
			}
		}
	}

	data := make(map[string]interface{})
	if secret != nil && secret.Data != nil {
		// There was a secret, it just didn't have the Key
		// we're looking for. We'll keep the data so it doesn't get erased.
		data = secret.Data
	}

	// didn't find the value
	if secretValue == "" {
		update = true
		if defaultValue != "" {
			// There was a default value, so we'll store that.
			secretValue = defaultValue
		} else if !IsInteractive() {
			// No terminal attached, so we can't ask the user for values.
			return "", errors.Errorf("no vault secret found at Path %q", path)
		} else {
			// Prompt the user for the value.
			secretValue = RequestStringFromUser("No value found in VaultClient at Path %q; please provide the value", path)
		}

		// User didn't provide a Key, so we'll set the value under "Key"
		if key == "" {
			key = "Key"
		}
	}

	if update {
		// Store the data in Vault so we'll have it next time.
		data[key] = secretValue
		_, err = client.Logical().Write(path, data)

		if err != nil {
			return "", errors.WithMessage(err, "could not persist provided secret in vault")
		}
	}

	return secretValue, nil
}

func IsInteractive() bool {
	return terminal.IsTerminal(int(os.Stdout.Fd()))
}
