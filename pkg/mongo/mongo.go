package mongo

import (
	"bufio"
	"context"
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
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
	DataFile     *string              `json:"dataFile,omitempty" yaml:"dataFile,omitempty"`
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
	Namespace   string `yaml:"namespace" json:"namespace"`
}

type MongoImportCommand struct {
	Conn      Connection
	DB        Database
	DataDir   string
	RebuildDB bool
	Log       *logrus.Entry
}

func (c MongoImportCommand) Execute() error {
	if c.Log == nil {
		c.Log = logrus.WithField("cmd", "MongoImportCommand")
	}

	pc, err := c.Conn.Prepare(c.Log)
	if err != nil {
		return err
	}

	wrapper, err := c.getMongoWrapper(c.DataDir, pc)
	if err != nil {
		return fmt.Errorf("error connecting to mongo: %v", err)
	}
	if pc.CleanUp != nil {
		defer pc.CleanUp()
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

var preparedConnectionMap = map[string]*preparedConnectionEntry{}
var preparedConnectionLock = sync.Mutex{}

type preparedConnectionEntry struct {
	PreparedConnection PreparedConnection
	handles            int
	cmd                *exec.Cmd
}

func GetPreparedConnection(log *logrus.Entry, c Connection) (PreparedConnection, error) {

	key := strings.Join([]string{
		fmt.Sprint(c.KubePort.Port),
		c.KubePort.ServiceName,
		c.Credentials.VaultPath,
		c.Credentials.AuthSource,
		c.Credentials.Username,
		c.Credentials.Password,
		c.DBName,
	}, "|")

	preparedConnectionLock.Lock()
	defer preparedConnectionLock.Unlock()

	entry, ok := preparedConnectionMap[key]
	if ok {
		entry.handles++
		return entry.PreparedConnection, nil
	}

	entry = &preparedConnectionEntry{
		handles: 1,
		PreparedConnection: PreparedConnection{
			Connection: c,
			CleanUp: func() {

			},
		},
	}

	if c.KubePort.Forward {
		log.Info("Creating new kubectl port-forward for connection to MongoDB")

		kubePort := c.KubePort.Port
		kubeService := c.KubePort.ServiceName
		entry.cmd = exec.Command("kubectl",
			"port-forward",
			"--namespace", c.KubePort.Namespace,
			kubeService,
			fmt.Sprintf("0:%d", kubePort))

		entry.cmd.Stderr = os.Stderr
		portFwdOut, _ := entry.cmd.StdoutPipe()

		reader := bufio.NewReader(portFwdOut)
		log.Debugf("port-forwarding mongo with service name '%s' and port '%d'", kubeService, kubePort)
		// Start it up
		err := entry.cmd.Start()
		if err != nil {
			return PreparedConnection{}, errors.Wrapf(err, "error starting kubernetes port forwarding to %s on port %d", kubeService, kubePort)
		}

		entry.PreparedConnection.CleanUp = func() {
			preparedConnectionLock.Lock()
			defer preparedConnectionLock.Unlock()
			entry.handles--
			if entry.handles == 0 {
				err := entry.cmd.Process.Signal(os.Kill)
				if err != nil {
					logrus.WithError(err).Error("kill of port forward kubectl failed")
				}
				delete(preparedConnectionMap, key)
			}
		}

		err = func() error {

			line, _, err := reader.ReadLine()
			if err == io.EOF {
				err := entry.cmd.Wait()
				return errors.Wrap(err, "kubectl port-forward failed")
			}
			if err != nil {
				return errors.Wrap(err, "read kubectl port-forward output")
			}
			matches := regexp.MustCompile(`Forwarding from ([^:]+):(\d+)`).FindStringSubmatch(string(line))
			if len(matches) < 3 {
				return errors.Errorf("port forward failed; kubectl said %q", line)
			}

			entry.PreparedConnection.Host = matches[1]
			entry.PreparedConnection.Port = matches[2]

			// Wait for port to be available
			for {
				log.Debugf("checking for success of kubectl port-forward to mongodb at %s:%s", entry.PreparedConnection.Host, entry.PreparedConnection.Host)
				conn, _ := net.DialTimeout("tcp", net.JoinHostPort(entry.PreparedConnection.Host, entry.PreparedConnection.Port), time.Second*30)
				if conn != nil {
					conn.Close()
					log.Infof("kubectl port-forward to mongodb is ready at %s:%s", entry.PreparedConnection.Host, entry.PreparedConnection.Port)
					break
				}
			}
			return nil
		}()

		if err != nil {
			return entry.PreparedConnection, err
		}

	}

	var err error

	switch c.Credentials.Type {
	case "password":
		entry.PreparedConnection.Credentials.Username, entry.PreparedConnection.Credentials.Password, err = getPasswordCredentials(log, c.Credentials)
	case "vault":
		entry.PreparedConnection.Credentials.Username, entry.PreparedConnection.Credentials.Password, err = getVaultCredentials(log, c.Credentials)
	default:
		return entry.PreparedConnection, fmt.Errorf("the type '%s' is not a supported credential type, must be 'vault' or 'password'", c.Credentials.Type)
	}
	if err != nil {
		return entry.PreparedConnection, errors.Wrap(err, "get credentials")
	}

	mongoOptions := options.Client().SetAuth(options.Credential{
		Username: entry.PreparedConnection.Credentials.Username,
		Password: entry.PreparedConnection.Credentials.Password,
	}).SetDirect(true).SetHosts([]string{fmt.Sprintf("%s:%s", entry.PreparedConnection.Host, entry.PreparedConnection.Port)})

	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	client, err := mongo.Connect(ctx, mongoOptions)
	if err != nil {
		return entry.PreparedConnection, errors.Wrap(err, "connect to mongo")
	}

	err = client.Ping(ctx, readpref.Primary())

	if err != nil {
		return entry.PreparedConnection, errors.Wrap(err, "ping mongo")
	}

	entry.PreparedConnection.Client = client

	preparedConnectionMap[key] = entry

	return entry.PreparedConnection, nil
}

//
// var defaultPortForwarder = &portForwardManager{
// 	entries: map[string]*forwardedPortEntry{},
// }
//
// type portForwardManager struct {
// 	mu      sync.Mutex
// 	entries map[string]*forwardedPortEntry
// }
//
// type forwardedPortEntry struct {
// 	port    string
// 	host    string
// 	cmd     *exec.Cmd
// 	handles int
// }
//
// type forwardedPort struct {
// 	port    string
// 	host    string
// 	release func()
// }
//
// func (p *portForwardManager) GetOrCreate(log *logrus.Entry, service string, namespace string, port int) (forwardedPort, error) {
//
// 	p.mu.Lock()
// 	defer p.mu.Unlock()
//
// 	key := fmt.Sprintf("%s|%s|%d", service, namespace, port)
//
// 	entry, ok := p.entries[key]
// 	if ok {
// 		log.Debug("Using existing kubectl port-forward for connection to MongoDB")
// 	} else {
// 		log.Info("Creating new kubectl port-forward for connection to MongoDB")
// 		entry = &forwardedPortEntry{}
// 		entry.cmd = exec.Command("kubectl",
// 			"port-forward",
// 			"--namespace", namespace,
// 			service,
// 			fmt.Sprintf("0:%d", port))
//
// 		entry.cmd.Stderr = os.Stderr
// 		portFwdOut, _ := entry.cmd.StdoutPipe()
//
// 		reader := bufio.NewReader(portFwdOut)
// 		log.Debugf("port-forwarding mongo with service name '%s' and port '%d'", service, port)
// 		// Start it up
// 		err := entry.cmd.Start()
// 		if err != nil {
// 			return forwardedPort{}, errors.Wrapf(err, "error starting kubernetes port forwarding to %s on port %d", service, port)
// 		}
//
// 		err = func() error {
//
// 			line, _, err := reader.ReadLine()
// 			if err == io.EOF {
// 				err := entry.cmd.Wait()
// 				return errors.Wrap(err, "kubectl port-forward failed")
// 			}
// 			if err != nil {
// 				return errors.Wrap(err, "read kubectl port-forward output")
// 			}
// 			matches := regexp.MustCompile(`Forwarding from ([^:]+):(\d+)`).FindStringSubmatch(string(line))
// 			if len(matches) < 3 {
// 				return errors.Errorf("port forward failed; kubectl said %q", line)
// 			}
//
// 			entry.PreparedConnection.Host = matches[1]
// 			entry.PreparedConnection.Port = matches[2]
//
// 			// Wait for port to be available
// 			for {
// 				log.Debugf("checking for success of kubectl port-forward to mongodb at %s:%s", entry.PreparedConnection.Host, entry.PreparedConnection.Port)
// 				conn, _ := net.DialTimeout("tcp", net.JoinHostPort(entry.PreparedConnection.Host, entry.PreparedConnection.Port), time.Second*30)
// 				if conn != nil {
// 					conn.Close()
// 					log.Infof("kubectl port-forward to mongodb is ready at %s:%s", entry.PreparedConnection.Host, entry.PreparedConnection.Port)
// 					break
// 				}
// 			}
// 			return nil
// 		}()
//
// 		p.entries[key] = entry
//
// 	}
//
// 	entry.handles++
// 	return forwardedPort{
// 		host: entry.PreparedConnection.Host,
// 		port: entry.PreparedConnection.Port,
// 		release: func() {
// 			p.mu.Lock()
// 			defer p.mu.Unlock()
// 			entry.handles--
// 			if entry.handles == 0 {
// 				err := entry.cmd.Process.Signal(os.Kill)
// 				if err != nil {
// 					logrus.WithError(err).Error("kill of port forward kubectl failed")
// 				}
// 				delete(p.entries, key)
// 			}
// 		},
// 	}, nil
//
// }

// Prepare returns a PreparedConnection which may have created a port-forward for mongo
// and will have ensured that the credentials are populated.
func (c Connection) Prepare(log *logrus.Entry) (PreparedConnection, error) {
	if c.KubePort.ServiceName != "" {
		c.KubePort.ServiceName = "svc/mongodb"
	}

	if c.KubePort.Port == 0 {
		c.KubePort.Port = 27017
	}

	return GetPreparedConnection(log, c)
	//
	// var err error
	//
	// switch c.Credentials.Type {
	// case "password":
	// 	c.Credentials.Username, c.Credentials.Password, err = getPasswordCredentials(log, c.Credentials)
	// case "vault":
	// 	c.Credentials.Username, c.Credentials.Password, err = getVaultCredentials(log, c.Credentials)
	// default:
	// 	return nil, fmt.Errorf("the type '%s' is not a supported credential type, must be 'vault' or 'password'", c.Credentials.Type)
	// }
	//
	// if !c.KubePort.Forward {
	// 	return &PreparedConnection{
	// 		Connection: c,
	// 		CleanUp:    func() {},
	// 	}, nil
	// }
	//
	// namespace := c.KubePort.Namespace
	// if namespace == "" {
	// 	namespace = "default"
	// }
	//
	// forwardedPort, err := defaultPortForwarder.GetOrCreate(log, svcName, namespace, svcPort)
	//
	// if err != nil {
	// 	return nil, err
	// }
	//
	// c.Port = forwardedPort.port
	// c.Host = forwardedPort.host
	//
	// return &PreparedConnection{
	// 	Connection: c,
	// 	CleanUp:    forwardedPort.release,
	// }, nil
}

type PreparedConnection struct {
	Connection
	Client  *mongo.Client
	CleanUp func()
}

func (i MongoImportCommand) getMongoWrapper(dataDir string, c PreparedConnection) (*mongoWrapper, error) {
	wrapper, err := newMongoWrapper(
		c.Host,
		c.Port,
		c.DBName,
		c.Credentials.Username,
		c.Credentials.Password,
		c.Credentials.AuthSource,
		dataDir,
		i.Log.WithField("typ", "mongoWrapper"))

	if err != nil {
		return nil, fmt.Errorf("could not get mongo wrapper: %v", err)
	}

	return wrapper, nil
}

func getPasswordCredentials(log *logrus.Entry, c CredentialProvider) (string, string, error) {
	log.Debug("getting mongo credentials using 'password' type")
	return c.Username, c.Password, nil
}

func getVaultCredentials(log *logrus.Entry, c CredentialProvider) (username string, password string, err error) {
	log.Debug("getting mongo credentials using 'vault' type")
	username = ""
	password = ""

	vaultToken := os.Getenv("VAULT_TOKEN")
	vaultAddr := os.Getenv("VAULT_ADDR")

	log.Debugf("getting vault client at '%s' with token '%s'", vaultAddr, vaultToken)

	vault, err := pkg.NewVaultLowlevelClient(vaultToken, vaultAddr)
	if err != nil {
		return
	}

	log.Debugf("getting credentials from vault using path '%s'", c.VaultPath)
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
