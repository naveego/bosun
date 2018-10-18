package internal

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/vault/api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh/terminal"
	"gopkg.in/yaml.v2"
)

type VaultLayout struct {
	Auth      map[string]map[string]interface{}
	Mounts    map[string]map[string]interface{}
	Resources map[string]map[string]interface{}
	Policies  map[string]map[string]interface{}
}

type VaultLayoutTemplateArgs struct {
	Cluster string
	Domain  string
	Values  map[string]interface{}
}

func LoadVaultLayoutFromFile(path string, templateArgs VaultLayoutTemplateArgs, client *api.Client) (VaultLayout, error) {
	var vl VaultLayout

	b, err := ioutil.ReadFile(path)
	if err != nil {
		return vl, err
	}

	yamlString, err := NewTemplateBuilder(path).
		WithKubeFunctions().
		WithVaultTemplateFunctions(client).
		WithTemplate(string(b)).
		BuildAndExecute(templateArgs)
	if err != nil {
		return vl, err
	}

	err = yaml.Unmarshal([]byte(yamlString), &vl)
	if err != nil {
		color.Red("Invalid yaml:")
		fmt.Println(yamlString)
		return vl, err
	}

	return vl, nil
}

func (v VaultLayout) Apply(client *api.Client) error {

	hadErrors := false

	recordError := func(log *logrus.Entry, data interface{}, err error) {
		log.WithField("data", data).WithError(err).Error()
		hadErrors = true
	}

	for path, data := range v.Auth {
		log := Log.WithField("@type", "Auth").WithField("path", path)
		mounts, err := client.Sys().ListAuth()
		if err != nil {
			return errors.Errorf("could not list items: %s", err)
		}

		if _, ok := mounts[strings.TrimRight(path, "/")+"/"]; ok {
			// Auth engine already mounted
			log.Debug("Auth method already exists.")
			continue
		}

		_, err = client.Logical().Write(fmt.Sprintf("sys/auth/%s", path), remap(data))
		if err != nil {
			recordError(log, data, err)
			continue
		}
		log.Info("Auth engine mounted.")
	}

	for path, data := range v.Mounts {
		log := Log.WithField("@type", "Mount").WithField("path", path)
		mounts, err := client.Sys().ListMounts()
		if err != nil {
			return errors.Errorf("could not list items: %s", err)
		}

		if _, ok := mounts[strings.TrimRight(path, "/")+"/"]; ok {
			// Auth engine already mounted
			log.Debug("Secret engine already mounted.")
			continue
		}
		_, err = client.Logical().Write(fmt.Sprintf("sys/mounts/%s", path), remap(data))
		if err != nil {
			recordError(log, data, err)
			continue
		}

		log.Info("Secret engine mounted.")
	}

	for path, data := range v.Resources {
		log := Log.WithField("@type", "Resource").WithField("path", path)

		u, err := url.Parse(path)
		if err != nil {
			recordError(log, data, errors.WithMessage(err, "resource path was invalid"))
			continue
		}

		path = u.Path
		mode := u.Query().Get("mode")

		switch mode {
		case "delete":
			_, err := client.Logical().Delete(path)
			if err != nil {
				recordError(log, "", errors.WithMessage(err, "delete failed"))
				continue
			}
			log.Info("Deleted resource.")
			continue
		case "insert", "create":
			secret, err := client.Logical().Read(path)
			if err != nil {
				recordError(log, "redacted", errors.WithMessage(err, "could not check if resource already exists, not safe to insert"))
				continue
			}
			if secret != nil {
				log.Warn("skipping because mode was insert and resource already exists")
				continue
			}
		case "update", "upsert":
			// no special action
		default:
		}

		_, err = client.Logical().Write(path, remap(data))
		if err != nil {
			recordError(log, data, err)
			continue
		}
		log.Info("Resource updated.")
	}

	for path, data := range v.Policies {
		log := Log.WithField("@type", "Policy").WithField("path", path)
		b, err := json.Marshal(remap(data))
		if err != nil {
			recordError(log, data, err)
			continue
		}
		err = client.Sys().PutPolicy(path, string(b))
		if err != nil {
			recordError(log, data, err)
			continue
		}
		log.Info("Policy updated.")
	}

	if hadErrors {
		return errors.New("Vault apply failed. See log for errors.")
	}

	return nil
}

func remap(m map[string]interface{}) map[string]interface{} {
	x := ensureJsonMarshallable(m)
	return x.(map[string]interface{})
}

func ensureJsonMarshallable(m interface{}) interface{} {

	switch v := m.(type) {
	case map[interface{}]interface{}:
		mapsi := map[string]interface{}{}
		for ki, vi := range v {
			if ks, ok := ki.(string); ok {
				mapsi[ks] = ensureJsonMarshallable(vi)
			} else {
				Log.WithField("ki", ki).WithField("v", v).Panicf("could not convert child key %v (of type %T) to string", ki, ki)
			}
		}
		return mapsi
	case map[string]interface{}:
		for ki, vi := range v {
			v[ki] = ensureJsonMarshallable(vi)
		}
		return v
	case []interface{}:
		for i := range v {
			v[i] = ensureJsonMarshallable(v[i])
		}
		return v
	default:
		return v
	}
}

func NewVaultLowlevelClient(token, vaultAddr string) (*api.Client, error) {

	log := Log.WithField("method", "NewVaultLowLevelClient")

	vaultConfig := api.DefaultConfig()
	vaultConfig.Address = vaultAddr

	err := vaultConfig.ReadEnvironment()
	if err != nil {
		return nil, errors.WithMessage(err, "Couldn't configure vault client")
	}

	if vaultConfig.Address == "" {
		log.Fatal("VaultClient address was not set (try setting VAULT_ADDR)")
	}

	vaultClient, err := api.NewClient(vaultConfig)
	if err != nil {
		log.WithError(err).Fatalf("couldn't create vault client")
	}

	if token == "" {
		token = os.Getenv("VAULT_TOKEN")
	}

	if token == "" {
		// Try to read the token from the user's most recent vault login.
		vaultAuthFile := os.ExpandEnv("$HOME/.vault-token")
		if vaultToken, err := ioutil.ReadFile(vaultAuthFile); err == nil {
			token = string(vaultToken)
		}
	}

	if token == "" {
		credentialBytes, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
		if err == nil {
			log.Debug("Found kubernetes token, attempting to authenticate.")
			credential := string(credentialBytes)

			role, ok := os.LookupEnv("VAULT_KUBERNETES_ROLE")
			if !ok {
				role = "devops"
			}

			secret, err := vaultClient.Logical().Write("/auth/kubernetes/login", map[string]interface{}{
				"role": role,
				"jwt":  credential,
			})

			if err != nil {
				return nil, errors.Errorf("kubernetes authentication failed using role %q: %s", role, err)
			}

			token, err = secret.TokenID()
			if err != nil {
				return nil, errors.WithMessage(err, "secret returned from kubernetes login did not have a token")
			}
		}
	}

	if token == "" {

		if terminal.IsTerminal(int(os.Stdout.Fd())) {
			log.Error("TTY attached, will not attempt to authenticate as EC2 instance.")
		} else {
			token, err = tryGetTokenUsingEC2Metadata(vaultClient)
			if err != nil {
				return nil, errors.WithMessage(err, "No token was provided in flags or environment, so attempted to use EC2 metadata. This failed.")
			}
		}
	}

	vaultClient.SetToken(token)
	if vaultClient.Token() == "" {
		log.Fatal("VaultClient token was not set (try setting VAULT_TOKEN)")
	}

	return vaultClient, nil
}

func tryGetTokenUsingEC2Metadata(vaultClient *api.Client) (string, error) {
	resp, err := http.Get("http://169.254.169.254/latest/dynamic/instance-identity/pkcs7")
	if err != nil {
		return "", fmt.Errorf("couldn't get EC2 metadata: %s", err)
	}
	if resp == nil {
		return "", errors.New("response was empty")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("attempt to get EC2 metadata returned bad status: %d - %s", resp.StatusCode, resp.Status)
	}
	defer resp.Body.Close()
	key, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("metadata from EC2 could not be read: %s", err)
	}

	noncePath := os.ExpandEnv("$HOME/.vault-aws-nonce")
	nonce, err := ioutil.ReadFile(noncePath)

	if os.IsNotExist(err) {
		u, _ := uuid.GenerateUUID()
		nonce = []byte(u)
		err = ioutil.WriteFile(noncePath, nonce, 0600)
		if err != nil {
			return "", fmt.Errorf("error saving nonce to %q: %s", noncePath, err)
		}
	} else if err != nil {
		return "", fmt.Errorf("error reading nonce from path %q: %s", noncePath, err)
	}

	secret, err := vaultClient.Logical().Write("Auth/aws/login", map[string]interface{}{
		"role":  "provisioner",
		"pkcs7": string(key),
		"nonce": string(nonce),
	})

	if err != nil {
		return "", fmt.Errorf("could not use EC2 metadata to login to vault: %s", err)
	}

	return secret.Auth.ClientToken, nil
}

// CreateWrappedAppRoleToken creates a wrapped logged in token for the provided appRole.
func CreateWrappedAppRoleToken(vaultClient *api.Client, appRole string) (string, error) {

	vault := vaultClient.Logical()

	secret, err := vault.Read(fmt.Sprintf("Auth/approle/role/%s/role-id", appRole))
	if err != nil {
		return "", fmt.Errorf("error reading role %q: %s", appRole, err)
	}
	if secret == nil {
		return "", fmt.Errorf("could not find role with name %q", appRole)
	}

	roleID := secret.Data["role_id"].(string)

	secret, err = vault.Write(fmt.Sprintf("Auth/approle/role/%s/secret-id", appRole), map[string]interface{}{})
	if err != nil {
		return "", fmt.Errorf("error creating secret for role %q: %s", appRole, err)
	}
	secretID := secret.Data["secret_id"].(string)

	vaultClient.SetWrappingLookupFunc(func(operation, path string) string {
		if strings.HasSuffix(path, "Auth/approle/login") {
			return "10m"
		}
		return ""
	})

	wrapped, err := vault.Write(fmt.Sprintf("Auth/approle/login"), map[string]interface{}{
		"role_id":   roleID,
		"secret_id": secretID,
	})
	if err != nil {
		return "", fmt.Errorf("error logging in using appRole %q: %s", appRole, err)
	}

	wrappedToken := wrapped.WrapInfo.Token

	return wrappedToken, nil
}
