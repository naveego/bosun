package pkg

import (
	"encoding/base64"
	"fmt"
	"github.com/hashicorp/vault/api"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"path/filepath"
)

type VaultInitializer struct {
	Client *api.Client
}

func (v VaultInitializer) InitNonProd() error {
	vaultClient := v.Client
	initialized, err := vaultClient.Sys().InitStatus()
	if err != nil {
		return err
	}

	if !initialized {
		_, _, err = v.initialize()
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("Vault at %q is already initialized.\n", vaultClient.Address())
	}

	err = v.Unseal("")
	if err != nil {
		return errors.Wrap(err, "unseal")
	}

	err = v.installPlugin()

	return err

}


func (v VaultInitializer) installPlugin()error {
	vaultClient := v.Client

	Log.Debug("Getting hash for JOSE...")

	joseSHA, err := NewCommand("kubectl exec vault-dev-0 cat /vault/plugins/jose-plugin.sha").RunOut()
	if err != nil {
		return err
	}

	Log.Debug("Registering JOSE...")
	err = vaultClient.Sys().RegisterPlugin(&api.RegisterPluginInput{
		Name:"jose",
		SHA256:joseSHA,
		Command:"jose-plugin",
	})

	if err != nil {
		return err
	}

	Log.Debug("JOSE plugin installed and registered.")
	return nil
}

func (v VaultInitializer) Unseal(path string) error {

	vaultClient := v.Client

	sealStatus, err := vaultClient.Sys().SealStatus()
	if err != nil {
		return err
	}
	if !sealStatus.Sealed {
		fmt.Printf("Vault at %q is already unsealed.\n", vaultClient.Address())
		return nil

	}

	var keys []string

	if path == "" {
		secretYaml, err := NewCommand("kubectl get secret vault-unseal-keys -o yaml").RunOut()
		if err != nil {
			return err
		}
		var secret map[string]interface{}
		err = yaml.Unmarshal([]byte(secretYaml), &secret)
		if err != nil {
			return err
		}
		m := secret["data"].(map[interface{}]interface{})
		for _, v := range m {
			shard, _ := base64.StdEncoding.DecodeString(v.(string))
			keys = append(keys, string(shard))
		}
	} else {
		files, _ := filepath.Glob(path +"/key*")
		Log.WithField("files", files).Debug("Found key files.")
		for _, file := range files {
			key, _ := ioutil.ReadFile(file)
			keys = append(keys, string(key))
		}
	}


	for k, v := range keys {
		fmt.Printf("Unsealing with key %v: %q\n", k, v)
		_, err = vaultClient.Sys().Unseal(v)
		if err != nil {
			return err
		}
	}

	return nil
}

func (v VaultInitializer) initialize() (keys []string, rootToken string, err error) {
	vaultClient := v.Client

	err = NewCommand("kubectl delete secret vault-unseal-keys --ignore-not-found=true").RunE()
	err = NewCommand("kubectl delete secret vault-root-token --ignore-not-found=true").RunE()
	if err != nil {
		return nil, "", err
	}

	initResp, err := vaultClient.Sys().Init(&api.InitRequest{
		SecretShares:1,
		SecretThreshold:1,
	})
	if err != nil {
		return nil, "", err
	}

	err = NewCommand("kubectl", "create", "secret", "generic", "vault-root-token", fmt.Sprintf("--from-literal=root=%s", initResp.RootToken)).RunE()
	if err != nil {
		return nil, "", err
	}

	for i, key := range initResp.Keys {
		fmt.Printf("Seal key %d: %q", i, key)

		err = NewCommand("kubectl", "create", "secret", "generic", "vault-unseal-keys", fmt.Sprintf("--from-literal=key%d=%s", i, key)).RunE()
		if err != nil {
			return nil, "", err
		}

		vaultClient.Sys().Unseal(key)
	}

	root := initResp.RootToken

	vaultClient.SetToken(root)

	_, err = vaultClient.Auth().Token().Create(&api.TokenCreateRequest{
		ID:"root",
		Policies:[]string{"root"},
	})


	return initResp.Keys, initResp.RootToken, err
}
