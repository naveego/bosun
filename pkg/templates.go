package pkg

import (
	"encoding/base64"
	"fmt"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Masterminds/sprig"
	"github.com/fatih/color"
	vault "github.com/hashicorp/vault/api"
	"github.com/imdario/mergo"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh/terminal"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

type TemplateHelper struct {
	TemplateValues TemplateValues
	VaultClient    *vault.Client
}

func (h *TemplateHelper) LoadFromYaml(out interface{}, globs ...string) (error) {

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
			color.Red("Invalid yaml in %s:", path)
			lines := strings.Split(yamlString, "\n")
			for i, line := range lines {
				if i == badLine {
					color.Red(line + "\n")
				} else {
					fmt.Println(yamlString)
				}
			}

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
	content string
}

func NewTemplateBuilder(name string) *TemplateBuilder {
	t := template.New(name).
		Funcs(sprig.TxtFuncMap()).
		Funcs(template.FuncMap{
			"exec": func(exe string, args ...string) (string, error) {
				out, err := NewCommand(exe, args...).RunOut()
				return out, err
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

func (t *TemplateBuilder) WithKubeFunctions() *TemplateBuilder {

	t.t = t.t.Funcs(template.FuncMap{
		"kube_server": func(cluster string) (string, error) {
			if cluster == "" {
				return "", errors.New("cluster parameter was not set")
			}

			o, err := NewCommand(fmt.Sprintf(`kubectl config view --raw -o jsonpath={.clusters[?(@.name=="%s")].cluster.server}'`, cluster)).RunOut()
			return o, err
		},
		"kube_ca_cert": func(cluster string) (string, error) {
			if cluster == "" {
				return "", errors.New("cluster parameter was not set")
			}

			b64, err := NewCommand(fmt.Sprintf(`kubectl config view --raw -o jsonpath={.clusters[?(@.name=="%s")].cluster.certificate-authority-data}'`, cluster)).RunOut()

			if err != nil {
				return "", err
			}

			b64 = strings.Trim(b64, "\"'")
			cert, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				return "", errors.Errorf("could not decode %q: %s", b64, err)
			}

			return string(cert), nil
		},
		"kube_service_token": func(cluster string, serviceAccount string) (string, error) {
			if cluster == "" {
				return "", errors.New("cluster parameter was not set")
			}

			o, err := NewCommand("kubectl", "--context", cluster, "get", "serviceaccounts", serviceAccount, "-o", "jsonpath={.secrets[0].name}").RunOut()
			if err != nil {
				return "", errors.Errorf("getting service account data for account %q in cluster %q: %s", serviceAccount, cluster, err)
			}

			o, err = NewCommand(fmt.Sprintf(`kubectl --context=%s get secrets %s -o jsonpath={.data.token}'`, cluster, o)).RunOut()
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

			action := getOrUpdateVaultSecretAction{
				path:         path,
				key:          key,
				defaultValue: defaultValue,
				client:       client,
			}

			return action.execute()
		},

		"bcryptedVaultSecret": func(path, key, plaintext string) (string, error) {
			action := getOrUpdateVaultSecretAction{
				path:         path,
				key:          key,
				defaultValue: plaintext,
				bcrypt:       true,
				client:       client,
			}

			return action.execute()
		},
	})

	return t
}

type getOrUpdateVaultSecretAction struct {
	path         string
	key          string
	defaultValue string
	replace      bool
	bcrypt       bool
	client       *vault.Client
}

func (g getOrUpdateVaultSecretAction) execute() (string, error) {

	var secretValue string
	client := g.client
	path := g.path
	key := g.key
	defaultValue := g.defaultValue

	update := g.replace

	secret, err := client.Logical().Read(path)
	if err != nil {
		return "", err
	}

	if secret != nil && secret.Data != nil {

		if key == "" && len(secret.Data) == 1 {
			// user didn't specify key, but there is only one
			// value, so we'll use that
			for _, v := range secret.Data {
				secretValue, _ = v.(string)
				break
			}
		} else {
			value, ok := secret.Data[key]
			if ok {
				// secret contained requested key
				secretValue, _ = value.(string)
			}
		}
	}

	data := make(map[string]interface{})
	if secret != nil && secret.Data != nil {
		// There was a secret, it just didn't have the key
		// we're looking for. We'll keep the data so it doesn't get erased.
		data = secret.Data
	}

	if g.bcrypt {
		// value is stored in bcrypted format
		var secretNotSet = secretValue == ""
		var secretHasChanged = bcrypt.CompareHashAndPassword([]byte(secretValue), []byte(defaultValue)) != nil
		if secretNotSet || secretHasChanged {
			b, _ := bcrypt.GenerateFromPassword([]byte(defaultValue), 15)
			secretValue = string(b)
			update = true
		}
	}

	// didn't find the value
	if secretValue == "" {
		update = true
		if defaultValue != "" {
			// There was a default value, so we'll store that.
			secretValue = defaultValue
		} else if !IsInteractive() {
			// No terminal attached, so we can't ask the user for values.
			return "", errors.Errorf("no vault secret found at path %q", path)
		} else {
			// Prompt the user for the value.
			secretValue = RequestStringFromUser("No value found in VaultClient at path %q; please provide the value", path)
		}

		// User didn't provide a key, so we'll set the value under "key"
		if key == "" {
			key = "key"
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
