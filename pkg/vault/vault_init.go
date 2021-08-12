package vault

import (
	"encoding/base64"
	"fmt"
	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/helper/consts"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/naveego/bosun/pkg/command"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/kube/kubeclient"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

type VaultInitializer struct {
	Client         *api.Client
	VaultNamespace string

	// Whether or not vault is using auto unseal.  If it is we don't need
	// to capture the unseal keys, since they will be sent to the target location
	// for storage.
	AutoUnseal 		bool

	// Overrides the prompting of the user to create  root token
	DisableDevRootTokenCreation bool

	DisableJoseInstall bool
}

func (v VaultInitializer) Init() error {
	vaultClient := v.Client
	initialized, err := vaultClient.Sys().InitStatus()
	if err != nil {
		return err
	}

	if !initialized {
		err = v.initialize()
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("Vault at %q is already initialized.\n", vaultClient.Address())
	}

	if !v.AutoUnseal {
		err = v.Unseal()
		if err != nil {
			return errors.Wrap(err, "unseal")
		}
	}

	if !v.DisableJoseInstall {
		err = v.InstallJose()
	}

	return err

}

func (v VaultInitializer) InstallJose() error {
	vaultClient := v.Client

	vaultNS := v.VaultNamespace
	if vaultNS == "" {
		vaultNS = "kube-system"
	}

	vaultPods, err := command.NewShellExe(fmt.Sprintf("kubectl get pods -l app=vault -n %s -o name", vaultNS)).RunOut()
	if err != nil {
		return err
	}

	for _, vaultPod := range strings.Split(vaultPods, "\n") {
		log := core.Log.WithField("pod", vaultPod)

		log.Infof("Getting hash for JOSE...")

		shaLine, err2 := command.NewShellExe(fmt.Sprintf("kubectl exec -n %s %s cat /vault/plugins/jose-plugin.sha", vaultNS, vaultPod)).RunOut()
		if err2 != nil {
			return err2
		}

		sha := strings.Split(shaLine, " ")[0]

		log.Infof("Registering JOSE using sha %s...", sha)
		err2 = vaultClient.Sys().RegisterPlugin(&api.RegisterPluginInput{
			Name:    "jose",
			SHA256:  sha,
			Type:    consts.PluginTypeSecrets,
			Command: "jose-plugin",
		})

		if err2 != nil {
			return err2
		}

		log.Info("JOSE plugin installed and registered.")
	}
	return nil
}

func (v VaultInitializer) Unseal() error {

	namespace := v.VaultNamespace
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

	secretYaml, getSecretErr := command.NewShellExe(fmt.Sprintf("kubectl get secret  -n %s vault-unseal-keys -o yaml", namespace)).RunOut()
	if getSecretErr != nil {
		return getSecretErr
	}
	var secret map[string]interface{}
	getSecretErr = yaml.Unmarshal([]byte(secretYaml), &secret)
	if getSecretErr != nil {
		return getSecretErr
	}
	m := secret["data"].(map[string]interface{})
	for _, d := range m {
		shard, _ := base64.StdEncoding.DecodeString(d.(string))
		keys = append(keys, string(shard))
	}

	for k, d := range keys {
		fmt.Printf("Unsealing with Key %d: %q\n", k, d)
		_, err = vaultClient.Sys().Unseal(d)
		if err != nil {
			return err
		}
	}

	return nil
}

func (v VaultInitializer) initialize() error {
	vaultClient := v.Client

	var initResp *api.InitResponse
	var initErr error

	log := core.Log

	log.Info("Initializing vault...")

	initResp, initErr = vaultClient.Sys().Init(&api.InitRequest{
		SecretShares:    1,
		SecretThreshold: 1,
		RecoveryThreshold: 1,
		RecoveryShares: 1,
	})
	if initErr != nil {
		return initErr
	}

	var rootToken string

	storeErr := func() error {

		kubeClient, err := kubeclient.GetKubeClient()
		if err != nil {
			return err
		}

		secretsClient := kubeClient.CoreV1().Secrets(v.VaultNamespace)

		if !v.AutoUnseal {
			log.Info("Storing unseal keys in k8s")

			unsealKeysSecret, err := secretsClient.Get("vault-unseal-keys", metav1.GetOptions{})
			if kerrors.IsNotFound(err) {
				unsealKeysSecret, err = secretsClient.Create(&v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vault-unseal-keys",
					},
					Type: v1.SecretTypeOpaque,
				})
				if err != nil {
					return err
				}
			} else if err != nil {
				return err
			}

			unsealKeysSecret.StringData = map[string]string{}

			for i, key := range initResp.Keys {
				fmt.Printf("Seal Key %d: %q", i, key)
				unsealKeysSecret.StringData[fmt.Sprintf("Key%d", i)] = key
			}

			_, err = secretsClient.Update(unsealKeysSecret)
			if err != nil {
				return errors.Wrap(err, "save unseal keys secret")
			}
		}

		log.Info("Storing root token in k8s")
		fmt.Printf("Initial root token: %s", initResp.RootToken)

		vaultRootTokenSecret, err := secretsClient.Get("vault-root-token", metav1.GetOptions{})
		if kerrors.IsNotFound(err) {
			vaultRootTokenSecret, err = secretsClient.Create(&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vault-root-token",
				},
				Type: v1.SecretTypeOpaque,
			})
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		vaultRootTokenSecret.StringData = map[string]string{
			"root": initResp.RootToken,
		}

		_, err = secretsClient.Update(vaultRootTokenSecret)
		if err != nil {
			return errors.Wrap(err, "save root token secret")
		}

		rootToken = initResp.RootToken

		return nil
	}()

	if storeErr != nil {
		return errors.Wrap(storeErr, "could not store unseal keys and initial root token; you will need to uninstall vault, destroy the pvc, and re-install")
	}

	if !v.AutoUnseal {
		log.Info("Unsealing vault")

		err := v.Unseal()
		if err != nil {
			return err
		}
	}

	if !v.DisableDevRootTokenCreation {
		vaultClient.SetToken(rootToken)

		createRootToken := cli.RequestConfirmFromUser("Should we create a root token named `root`")

		if createRootToken {
			log.Info("Creating token `root` (DELETE THIS TOKEN IN PRODUCTION!)")

			_, err := vaultClient.Auth().Token().Create(&api.TokenCreateRequest{
				ID:       "root",
				Policies: []string{"root"},
			})
			if err != nil {
				return err
			}

		}
	}

	log.Info("Init completed")

	return nil
}
