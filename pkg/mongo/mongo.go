package mongo

import (
	"bufio"
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"time"
)

type disposeFunc func()

// Database defines data to be imported into the target mongo database.
// It is the combination of a connection, collection definition, and data.
type Database struct {
	Collections map[string]*CollectionInfo `json:"collections" yaml:"collections"`
}

// CollectionInfo defines a collection in Mongo.  For creating a capped collection
// you can specify a size.
type CollectionInfo struct {
	IsCapped     bool                 `json:"isCapped" yaml:"isCapped"`
	Drop         bool                 `json:"drop" yaml:"drop"`
	MaxBytes     *int                 `json:"maxBytes,omitempty" yaml:"maxBytes,omitempty"`
	MaxDocuments *int                 `json:"maxDocuments,omitempty" yaml:"maxDocuments,omitempty"`
	Indexes      map[string]IndexInfo `json:"indexes,omitempty" yaml:"indexes,omitempty"`
	Data         *string              `json:"dataFile,omitempty" yaml:"dataFile,omitempty"`
}

type IndexInfo struct {
	Unique      bool           `json:"unique" yaml:"unique"`
	Sparse      bool           `json:"sparse" yaml:"sparse"`
	ExpireAfter *int           `json:"expireAfter" yaml:"expireAfter"`
	Fields      map[string]int `json:"fields" yaml:"fields"`
}

// Connection defines a mongo connection. It also has support for access mongo databases
// running inside a kubernetes cluster using the `port-forward` command, as well as
// credential support using vault.
type Connection struct {
	DBName      string             `yaml:"dbName" json:"dbName"`
	Host        string             `yaml:"host" json:"host"`
	Port        string             `yaml:"port" json:"port"`
	KubePort    KubePortForward    `yaml:"kubePort" json:"kubePort"`
	Credentials CredentialProvider `yaml:"credentials" json:"credentials"`
}

// CredentialProvider defines how the connection should obtain its credentials
type CredentialProvider struct {
	Type       string `yaml:"type" json:"type"`
	Username   string `yaml:"username,omitempty" json:"username,omitempty"`
	Password   string `yaml:"password,omitempty" json:"password,omitempty"`
	VaultPath  string `yaml:"vaultPath,omitempty" json:"vaultPath,omitempty"`
	AuthSource string `yaml:"authSource,omitempty" json:"authSource,omitempty"`
}

// KubePortForward defines whether or not we need to tunnel into Kuberetes, and what port to use.
type KubePortForward struct {
	Forward     bool   `yaml:"forward" json:"forward"`
	ServiceName string `yaml:"serviceName" json:"serviceName"`
	Port        int    `yaml:"port" json:"port"`
}

type MongoImportCommand struct {
	Conn      Connection
	DB        Database
	DataDir   string
	RebuildDB bool
}

func (c MongoImportCommand) Execute() error {
	wrapper, dispose, err := getMongoWrapper(c.DataDir, c.Conn)
	if err != nil {
		return fmt.Errorf("error connecting to mongo: %v", err)
	}
	if dispose != nil {
		defer dispose()
	}

	for colName, col := range c.DB.Collections {
		// if we are forcing a rebuild of the database
		// then we need to set the Drop flag.
		if c.RebuildDB {
			col.Drop = true
		}

		err = wrapper.Import(colName, *col)
		if err != nil {
			logrus.Warnf("could not import collection %s: %v", colName, err)
		}
	}

	return nil
}

// ImportDatabase imports a collection into a database
func ImportDatabase(conn Connection, db Database, dataDir string, rebuildDb bool) error {
	cmd := MongoImportCommand{
		Conn:      conn,
		DB:        db,
		DataDir:   dataDir,
		RebuildDB: rebuildDb,
	}

	return cmd.Execute()
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
		logrus.Info("using kubectl port-forward for connection to MongoDB")

		svcName := "svc/mongodb"
		if c.KubePort.ServiceName != "" {
			svcName = c.KubePort.ServiceName
		}

		svcPort := 27017
		if c.KubePort.Port >= 0 {
			svcPort = c.KubePort.Port
		}
		portFwdCmd = exec.Command("kubectl", "port-forward", svcName, fmt.Sprintf("0:%d", svcPort))

		portFwdCmd.Stderr = os.Stderr
		portFwdOut, _ := portFwdCmd.StdoutPipe()

		reader := bufio.NewReader(portFwdOut)
		logrus.Debugf("port-forwarding mongo with service name '%s' and port '%d'", svcName, svcPort)
		// Start it up
		err = portFwdCmd.Start()
		if err != nil {
			return nil, nil, fmt.Errorf("error starting kuberenetes port forwarding to %s on port %d: %v", svcName, svcPort, err)
		}

		cleanUp = func() {
			if portFwdCmd != nil {
				e := portFwdCmd.Process.Signal(os.Kill)
				if e != nil {
					os.Stderr.WriteString(e.Error())
				}
			}
		}

		line, _, err := reader.ReadLine()
		if err == io.EOF {
			cleanUp()
			err := portFwdCmd.Wait()
			return nil, nil, errors.Wrap(err, "kubectl port-forward failed")
		}
		if err != nil {
			return nil, cleanUp, errors.Wrap(err, "read kubectl port-forward output")
		}
		matches := regexp.MustCompile(`Forwarding from ([^:]+):(\d+)`).FindStringSubmatch(string(line))
		if len(matches) < 3 {
			return nil, cleanUp, errors.Errorf("port forward failed; kubectl said %q", line)
		}

		c.Host = matches[1]
		c.Port = matches[2]

		// Wait for port to be available
		for {
			logrus.Infof("checking for success of kubectl port-forward to mongodb at %s:%s", c.Host, c.Port)
			conn, _ := net.DialTimeout("tcp", net.JoinHostPort(c.Host, c.Port), time.Second*30)
			if conn != nil {
				conn.Close()
				logrus.Infof("kubectl port-forward to mongodb is ready at %s:%s", c.Host, c.Port)
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

	if loginSecret == nil {
		err = fmt.Errorf("could not get credentials from vault, try running 'vault read %s' for more information", c.VaultPath)
		return
	}

	username = loginSecret.Data["username"].(string)
	password = loginSecret.Data["password"].(string)
	return
}
