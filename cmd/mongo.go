package cmd

import (
	"fmt"
	"github.com/naveego/bosun/pkg/mongo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

// mongoCmd represents the git command
var mongoCmd = &cobra.Command{
	Use:   "mongo",
	Short: "Commands for working with MongoDB.",
}

func init() {
	mongoCmd.AddCommand(mongoImportCmd)

	rootCmd.AddCommand(mongoCmd)
}

var mongoImportCmd = &cobra.Command{
	Use:          "import",
	Args:         cobra.ExactArgs(1),
	Short:        "Import data into mongo database",
	Example:      "mongo import data.yaml",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dataFileName := args[0]
		logrus.Debugf("loading data into mongo from '%s'", dataFileName)

		dataFile, err := ioutil.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("could not read file '%s': %v", dataFileName, err)
		}

		logrus.Debugf("parsing file '%s'", dataFileName)
		i := mongo.Import{}
		err = yaml.Unmarshal(dataFile, &i)
		if err != nil {
			return fmt.Errorf("could not read file as yaml '%s': %v", dataFileName, err)
		}

		logrus.Debug("importing file")
		return mongo.ImportData(i)
	},
}
