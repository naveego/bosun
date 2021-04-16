package vault

import (
	"encoding/json"
	"fmt"
	"github.com/Azure/go-autorest/autorest/to"
	vault "github.com/hashicorp/vault/api"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/pkg/errors"
	"text/template"
	"time"
)

func TemplateFuncs(client *vault.Client) template.FuncMap {
	return template.FuncMap{


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
	}
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
		} else if !cli.IsInteractive() {
			// No terminal attached, so we can't ask the user for values.
			return "", errors.Errorf("no vault secret found at Dir %q", path)
		} else {
			// Prompt the user for the value.
			secretValue = cli.RequestStringFromUser("No value found in VaultClient at Dir %q; please provide the value", path)
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