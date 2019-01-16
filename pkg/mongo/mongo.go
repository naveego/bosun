package mongo

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/sirupsen/logrus"
	"net"
	"os"
	"os/exec"
	"time"
)

type disposeFunc func()

// Database defines data to be imported into the target mongo database.
// It is the combination of a connection, collection definition, and data.
type Database struct {
	Collections map[string]CollectionInfo `json:"collections" yaml:"collections"`
}

// CollectionInfo defines a collection in Mongo.  For creating a capped collection
// you can specify a size.
type CollectionInfo struct {
	IsCapped     bool    `json:"isCapped" yaml:"isCapped"`
	MaxBytes     *int    `json:"maxBytes,omitempty" yaml:"maxBytes,omitempty"`
	MaxDocuments *int    `json:"maxDocuments,omitempty" yaml:"maxDocuments,omitempty"`
	Data         *string `json:"dataFile,omitempty" yaml:"dataFile,omitempty"`
}

// Connection defines a mongo connection. It also has support for access mongo databases
// running inside a kubernetes cluster using the `port-forward` command, as well as
// credential support using vault.
type Connection struct {
	DBName      string
	Host        string
	Port        string
	KubePort    KubePortForward
	Credentials CredentialProvider
}

// CredentialProvider defines how the connection should obtain its credentials
type CredentialProvider struct {
	Type       string
	Username   string
	Password   string
	VaultPath  string
	AuthSource string
}

// KubePortForward defines whether or not we need to tunnel into Kuberetes, and what port to use.
type KubePortForward struct {
	Forward     bool
	ServiceName string
	Port        int
}

// ImportDatabase imports a collection into a database
func ImportDatabase(conn Connection, db Database, dataDir string) error {
	wrapper, dispose, err := getMongoWrapper(dataDir, conn)
	if err != nil {
		return fmt.Errorf("error connecting to mongo: %v", err)
	}
	if dispose != nil {
		defer dispose()
	}

	for colName, col := range db.Collections {
		err = wrapper.Import(colName, col)
		if err != nil {
			logrus.Warnf("could not import collection %s: %v", colName, err)
		}
	}

	return nil
}

func getMongoWrapper(dataDir string, c Connection) (*mongoWrapper, disposeFunc, error) {
	var portFwdCmd *exec.Cmd
	var err error
	var cleanUp disposeFunc

	defer func() {
		if cleanUp == nil && portFwdCmd != nil {
			e := portFwdCmd.Process.Signal(os.Kill)
			if e != nil {
				os.Stderr.WriteString(e.Error())
			}
		}
	}()

	// check to see if we need to port forward the connection
	if c.KubePort.Forward {
		logrus.Debug("using kubectl port-forward for connection to MongoDB")

		svcName := "svc/mongodb"
		if c.KubePort.ServiceName != "" {
			svcName = c.KubePort.ServiceName
		}

		svcPort := 27017
		if c.KubePort.Port >= 0 {
			svcPort = c.KubePort.Port
		}
		portFwdCmd = exec.Command("kubectl", "port-forward", svcName, fmt.Sprintf("%d", svcPort))

		logrus.Debugf("port-forwarding mongo with service name '%s' and port '%d'", svcName, svcPort)
		// Start it up
		err = portFwdCmd.Start()
		if err != nil {
			return nil, nil, fmt.Errorf("error starting kuberenetes port forwarding to %s on port %d: %v", svcName, svcPort, err)
		}

		// Wait for port to be available
		for {
			conn, _ := net.DialTimeout("tcp", net.JoinHostPort("", fmt.Sprintf("%d", svcPort)), time.Second*30)
			if conn != nil {
				conn.Close()
				logrus.Debug("kubectl port-forward to mongodb is ready")
				break
			}
		}
	}

	username := ""
	password := ""

	switch c.Credentials.Type {
	case "password":
		username, password, err = getPasswordCredentials(c.Credentials)
	case "vault":
		username, password, err = getVaultCredentials(c.Credentials)
	default:
		return nil, nil, fmt.Errorf("the type '%s' is not a supported credential type, must be 'vault' or 'password'", c.Credentials.Type)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("could not get credentials: %v", err)
	}

	wrapper, err := newMongoWrapper(
		c.Host,
		c.Port,
		c.DBName,
		username,
		password,
		c.Credentials.AuthSource,
		dataDir)

	if err != nil {
		return nil, nil, fmt.Errorf("could not get mongo wrapper: %v", err)
	}

	cleanUp = func() {
		if portFwdCmd != nil {
			e := portFwdCmd.Process.Signal(os.Kill)
			if e != nil {
				os.Stderr.WriteString(e.Error())
			}
		}
	}

	return wrapper, cleanUp, nil
}

func getPasswordCredentials(c CredentialProvider) (string, string, error) {
	logrus.Debug("getting mongo credentials using 'password' type")
	return c.Username, c.Password, nil
}

func getVaultCredentials(c CredentialProvider) (username string, password string, err error) {
	logrus.Debug("getting mongo credentials using 'vault' type")
	username = ""
	password = ""

	vaultToken := os.Getenv("VAULT_TOKEN")
	vaultAddr := os.Getenv("VAULT_ADDR")

	logrus.Debugf("getting vault client at '%s' with token '%s'", vaultAddr, vaultToken)

	vault, err := pkg.NewVaultLowlevelClient(vaultToken, vaultAddr)
	if err != nil {
		return
	}

	logrus.Debugf("getting credentials from vault using path '%s'", c.VaultPath)
	loginSecret, err := vault.Logical().Read(c.VaultPath)
	if err != nil {
		return
	}

	username = loginSecret.Data["username"].(string)
	password = loginSecret.Data["password"].(string)
	return
}
