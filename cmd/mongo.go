package cmd

import (
	"fmt"
	"github.com/naveego/bosun/pkg/mongo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"path/filepath"
	"strings"
)

const (
	ArgMongoDatabase        = "mongo-database"
	ArgMongoHost            = "host"
	ArgMongoPort            = "port"
	ArgMongoVaultAuth       = "vault-auth"
	ArgMongoUsername        = "username"
	ArgMongoPassword        = "password"
	ArgMongoAuthSource      = "auth-source"
	ArgMongoKubePortForward = "kube-port-forward"
	ArgMongoKubePort        = "kube-port"
	ArgMongoKubeSvcName     = "kube-service-name"
)

// mongoCmd represents the git command
var mongoCmd = &cobra.Command{
	Use:   "mongo",
	Short: "Commands for working with MongoDB.",
}

func init() {
	mongoCmd.AddCommand(mongoImportCmd)

	mongoImportCmd.Flags().String(ArgMongoDatabase, "", "The name of the database that will updated by this operation. If not set, the name of the file is used without the file extension.")
	mongoImportCmd.Flags().String(ArgMongoHost, "127.0.0.1", "The host address for connecting to the MongoDB server. (Default: 127.0.0.1)")
	mongoImportCmd.Flags().String(ArgMongoPort, "27017", "The port for connecting to the MongoDB server. (Default: 27017)")
	mongoImportCmd.Flags().String(ArgMongoVaultAuth, "", "The database credentials path to use with vault.  Setting this supersedes using username and password.")
	mongoImportCmd.Flags().String(ArgMongoUsername, "", "The username to use when connecting to Mongo")
	mongoImportCmd.Flags().String(ArgMongoPassword, "", "The password to use when connecting to Mongo")
	mongoImportCmd.Flags().String(ArgMongoAuthSource, "admin", "The authSource to use when validating the credentials")
	mongoImportCmd.Flags().Bool(ArgMongoKubePortForward, false, "Whether or not to use kubectl port-forward command")
	mongoImportCmd.Flags().Int(ArgMongoKubePort, 27017, "The port to use for mapping kubectl port-forward to your local host. Only used with --kube-port-forward")
	mongoImportCmd.Flags().String(ArgMongoKubeSvcName, "svc/mongodb", "Sets the kubernetes service name to use for forwarding. Only used with --kube-port-foward")

	rootCmd.AddCommand(mongoCmd)
}

var mongoImportCmd = &cobra.Command{
	Use:     "import",
	Args:    cobra.ExactArgs(1),
	Short:   "Import a Mongo database",
	Example: "mongo import db.yaml",
	//SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		viper.BindPFlags(cmd.Flags())

		dataFileName := args[0]
		logrus.Debugf("loading data into mongo from '%s'", dataFileName)

		dataFile, err := ioutil.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("could not read file '%s': %v", dataFileName, err)
		}

		logrus.Debugf("parsing file '%s'", dataFileName)
		db := mongo.Database{}
		err = yaml.Unmarshal(dataFile, &db)
		if err != nil {
			return fmt.Errorf("could not read file as yaml '%s': %v", dataFileName, err)
		}

		dataDir := filepath.Dir(dataFileName)

		conn := getConnection(dataFileName)

		logrus.Debug("importing file")
		return mongo.ImportDatabase(conn, db, dataDir)
	},
}

func getConnection(inputFile string) mongo.Connection {
	dbName := viper.GetString(ArgMongoDatabase)
	if dbName == "" {
		b := filepath.Base(inputFile)
		ext := filepath.Ext(b)
		dbName = strings.TrimSuffix(b, ext)
	}

	vaultCredPath := viper.GetString(ArgMongoVaultAuth)
	credType := "password"

	if vaultCredPath != "" {
		credType = "vault"
	}

	return mongo.Connection{
		DBName: dbName,
		Host:   viper.GetString(ArgMongoHost),
		Port:   viper.GetString(ArgMongoPort),
		KubePort: mongo.KubePortForward{
			Forward:     viper.GetBool(ArgMongoKubePortForward),
			Port:        viper.GetInt(ArgMongoKubePort),
			ServiceName: viper.GetString(ArgMongoKubeSvcName),
		},
		Credentials: mongo.CredentialProvider{
			Type:       credType,
			Username:   viper.GetString(ArgMongoUsername),
			Password:   viper.GetString(ArgMongoPassword),
			VaultPath:  viper.GetString(ArgMongoVaultAuth),
			AuthSource: viper.GetString(ArgMongoAuthSource),
		},
	}

}
