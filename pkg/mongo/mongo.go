package mongo

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/sirupsen/logrus"
	"gopkg.in/mgo.v2"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

type disposeFunc func()

// Import defines data to be imported into the target mongo database.
// It is the combination of a connection, collection definition, and data.
type Import struct {
	Database    string       `json"db" yaml:"db"`
	Connection  Connection   `json:"connection" yaml:"connection"`
	Collections []Collection `json:"collections" yaml:"collections"`
}

// Collection defines a collection in Mongo.  For creating a capped collection
// you can specify a size.
type Collection struct {
	Name         string                   `json:"name" yaml:"name"`
	IsCapped     *bool                    `json:"isCapped,omitempty" yaml:"isCapped,omitempty"`
	MaxBytes     *int                     `json:"maxBytes,omitempty" yaml:"maxBytes,omitempty"`
	MaxDocuments *int                     `json:"maxDocuments,omitempty" yaml:"maxDocuments,omitempty"`
	Data         []map[string]interface{} `json:"data,omitempty" yaml:",flow,omitempty"`
}

// Connection defines a mongo connection. It also has support for access mongo databases
// running inside a kubernetes cluster using the `port-forward` command, as well as
// credential support using vault.
type Connection struct {
	Host        *string             `json:"host,omitempty" yaml:"host,omitempty"`
	Port        *int                `json:"port,omitempty" yaml:"port,omitempty""`
	KubePort    *KubePortForward    `json:"kubePort,omitempty" yaml:"kubePort,omitempty"`
	Credentials *CredentialProvider `json:"credentials,omitempty" yaml:"credentials,omitempty"`
}

// CredentialProvider defines how the connection should obtain its credentials
type CredentialProvider struct {
	Type       string  `json:"type" yaml:"type"`
	Username   *string `json:"username,omitempty" yaml:"username,omitempty"`
	Password   *string `json:"password,omitempty" yaml:"password,omitempty"`
	VaultPath  *string `json:"vaultPath,omitempty" yaml:"vaultPath,omitempty"`
	AuthSource *string `json:"authSource,omitempty" yaml:"authSource,omitempty"`
}

// KubePortForward defines whether or not we need to tunnel into Kuberetes, and what port to use.
type KubePortForward struct {
	Forward     bool    `json:"forward" yaml:"forward"`
	ServiceName *string `json:"serviceName,omitempty" yaml:"serviceName,omitempty"`
	Port        *int    `json:"port,omitempty" yaml:"port,omitempty"`
}

// ImportData imports data into a database
func ImportData(i Import) error {
	session, dispose, err := getMongoConnection(i.Database, i.Connection)
	if err != nil {
		return fmt.Errorf("error connecting to mongo: %v", err)
	}
	if dispose != nil {
		defer dispose()
	}

	wrapper := &mongoWrapper{session}

	for _, col := range i.Collections {
		err = wrapper.DropCollection(col.Name)
		if err != nil {
			return fmt.Errorf("error dropping collection '%s': %v", col.Name, err)
		}

		err = wrapper.InsertData(col.Name, col.Data)
		if err != nil {
			return fmt.Errorf("error inserting data into collection '%s': %v", col.Name, err)
		}
	}

	return nil
}

func getMongoConnection(dbName string, c Connection) (*mgo.Session, disposeFunc, error) {
	var session *mgo.Session
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
	if c.KubePort != nil && c.KubePort.Forward {
		logrus.Debug("using kubectl port-forward for connection to MongoDB")

		svcName := "svc/mongodb"
		if c.KubePort.ServiceName != nil {
			svcName = *c.KubePort.ServiceName
		}

		svcPort := 27017
		if c.KubePort.Port != nil {
			svcPort = *c.KubePort.Port
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
	authSource := "admin"

	// resolve credentials if necessary
	if c.Credentials != nil {
		credType := strings.ToLower(c.Credentials.Type)
		switch credType {
		case "":
		case "password":
			username, password, err = getPasswordCredentials(c.Credentials)
		case "vault":
			username, password, err = getVaultCredentials(c.Credentials)
		default:
			return nil, nil, fmt.Errorf("the type '%s' is not a supported credential type, must be 'vault' or 'password'", credType)
		}

		if err != nil {
			return nil, nil, fmt.Errorf("could not get credentials: %v", err)
		}

		if c.Credentials.AuthSource != nil {
			authSource = *c.Credentials.AuthSource
		}
	}

	host := "127.0.0.1"
	port := 27017

	if c.Host != nil {
		host = *c.Host
	}

	if c.Port != nil {
		port = *c.Port
	}

	hostAndPort := fmt.Sprintf("%s:%d", host, port)
	dialInfo := &mgo.DialInfo{
		Database: dbName,
		Addrs:    []string{hostAndPort},
		Direct:   true,
		Source:   authSource,
		Username: username,
		Password: password,
	}

	session, err = mgo.DialWithInfo(dialInfo)
	if err != nil {
		return nil, nil, fmt.Errorf("could not connect to mongodb: %v", err)
	}

	cleanUp = func() {
		if portFwdCmd != nil {
			e := portFwdCmd.Process.Signal(os.Kill)
			if e != nil {
				os.Stderr.WriteString(e.Error())
			}
		}

		session.Close()
	}

	return session, cleanUp, nil
}

func getPasswordCredentials(c *CredentialProvider) (string, string, error) {
	logrus.Debug("getting mongo credentials using 'password' type")
	return *c.Username, *c.Password, nil
}

func getVaultCredentials(c *CredentialProvider) (username string, password string, err error) {
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

	logrus.Debugf("getting credentials from vault using path '%s'", *c.VaultPath)
	loginSecret, err := vault.Logical().Read(*c.VaultPath)
	if err != nil {
		return
	}

	username = loginSecret.Data["username"].(string)
	password = loginSecret.Data["password"].(string)
	return
}
