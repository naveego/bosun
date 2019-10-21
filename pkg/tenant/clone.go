package tenant

import (
	"bytes"
	"context"
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/naveego/bosun/pkg/cli"
	bmongo "github.com/naveego/bosun/pkg/mongo"
	"github.com/naveego/bosun/pkg/util"
	"github.com/pkg/errors"
	"github.com/rs/xid"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Plan struct {
	FromTenant               string             `yaml:"fromTenant"`
	ToTenant                 string             `yaml:"toTenant"`
	MongoConnection          bmongo.Connection  `yaml:"mongoConnection"`
	GoBetweenMongoConnection *bmongo.Connection `yaml:"goBetweenMongoConnection,omitempty"`
	FromAgents               []Agent            `yaml:"fromAgents"`
	ToAgents                 []Agent            `yaml:"toAgents"`
	AgentMapping             map[string]string  `yaml:"agentMapping"`
}

type Agent struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

type Planner struct {
	Dir  string
	Plan *Plan
	log  *logrus.Entry
}

func NewPlanner(dir string, log *logrus.Entry) (*Planner, error) {
	p := &Planner{
		Dir: dir,
		log: log,
	}
	err := p.Load()
	if err != nil {
		return nil, err
	}

	err = p.Save()
	return p, err
}

const (
	PlanFileName = "plan.yaml"
	DataDir      = "data"
)

func (p *Planner) getPlanPath() string {
	return filepath.Join(p.Dir, PlanFileName)
}

func (p *Planner) Load() error {
	path := p.getPlanPath()
	var plan *Plan
	err := util.LoadYaml(path, &plan)
	if err != nil {
		if os.IsNotExist(err) {
			plan = &Plan{}
		} else {
			return err
		}
	}
	p.Plan = plan
	return nil
}

func (p *Planner) Save() error {
	err := os.MkdirAll(p.Dir, 0770)
	if err != nil {
		return err
	}
	path := p.getPlanPath()
	err = util.SaveYaml(path, p.Plan)
	return err
}

func (p *Planner) Dump() {
	b, err := yaml.Marshal(p.Plan)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(string(b))
	}
}

func (p *Planner) EditPlan() error {
	for {
		path := p.getPlanPath()
		err := cli.Edit(path)
		if err != nil {
			return err
		}

		err = p.Load()
		if err != nil {
			return err
		}

		var more bool

		beforeValidation, _ := yaml.Marshal(p.Plan)

		validationErr := p.ValidatePlan()

		afterValidation, _ := yaml.Marshal(p.Plan)

		if validationErr != nil {
			confirm := &survey.Confirm{
				Message: "Plan is invalid. Do you want to edit again?",
			}
			err = survey.AskOne(confirm, &more)
			if err != nil {
				return err
			}
		} else if !bytes.Equal(beforeValidation, afterValidation) {
			confirm := &survey.Confirm{
				Message: "Validation updated the plan; do you want to edit again?",
			}
			err = survey.AskOne(confirm, &more)
			if err != nil {
				return err
			}
		}

		if !more {
			return validationErr
		}
	}
}

func (p *Planner) ValidatePlan() error {
	p.log.Info("Validating plan...")
	errs := validationErrs{}
	plan := p.Plan
	if plan.FromTenant == "" {
		errs = errs.With("fromTenant must be set")
	}
	if plan.ToTenant == "" {
		errs = errs.With("toTenant must be set")
	}

	p.log.Info("Validating Metabase MongoDB connection...")
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)

	metabaseConnection, err := bmongo.GetPreparedConnection(p.log, plan.MongoConnection)
	if err != nil {
		errs = errs.With("metabase mongoConnection is invalid: %+v", err)
	} else {
		names, listErr := metabaseConnection.Client.ListDatabaseNames(ctx, bson.D{})
		if listErr != nil {
			errs = errs.With("metabase mongoConnection is invalid: %+v", listErr)
		} else {
			p.log.Infof("Metabase MongoDB connection OK (found %d databases).", len(names))
		}
	}

	var goBetweenConnection bmongo.PreparedConnection
	if plan.GoBetweenMongoConnection != nil {
		goBetweenConnection, err = bmongo.GetPreparedConnection(p.log, *plan.GoBetweenMongoConnection)
		if err != nil {
			err = errors.Wrap(err, "create go-between mongodb connection")
		}
	} else {
		goBetweenConnection = metabaseConnection
	}

	p.log.Info("Validating Go-between MongoDB connection...")
	if err != nil {
		errs = errs.With("go-between mongoConnection is invalid: %s", err)
	} else {
		goBetweenDB := goBetweenConnection.Client.Database("go-between")
		p.log.Infof("Getting agents from %q...", plan.FromTenant)
		if plan.FromTenant != "" {
			plan.FromAgents, err = getAgents(plan.FromTenant, goBetweenDB)
			if err != nil {
				errs = errs.With("could not get from agents: %s", err)
			}
		}
		p.log.Infof("Getting agents from %q...", plan.ToTenant)
		if plan.ToTenant != "" {
			plan.ToAgents, err = getAgents(plan.ToTenant, goBetweenDB)
			if err != nil {
				errs = errs.With("could not get to agents: %s", err)
			}
		}
	}

	p.log.Info("Validating agent mappings...")
	for _, fromAgent := range plan.FromAgents {
		toAgentID := plan.AgentMapping[fromAgent.ID]
		if toAgentID == "" {
			plan.AgentMapping[fromAgent.ID] = ""
			errs = errs.With("agentMapping does not have a mapping for %q (%s)", fromAgent.ID, fromAgent.Name)
		} else {
			var found bool
			for _, toAgent := range plan.ToAgents {
				found = toAgent.ID == toAgentID
				if found {
					break
				}
			}
			if !found {
				errs = errs.With("agent %q (%s) is mapped to %q, which does not exist", fromAgent.ID, fromAgent.Name, toAgentID)
			}
		}
	}

	p.Dump()

	for _, e := range errs {
		fmt.Printf("%s%s\n", color.RedString("Invalid: "), e)
	}

	err = p.Save()
	if err != nil {
		return errors.Wrapf(err, "save after validate (errors: %s)", strings.Join(errs, "; "))
	}

	if len(errs) == 0 {
		color.Green("\nPlan is valid.\n")
		return nil
	}

	return errors.New("plan is invalid")
}

func (p *Planner) PerformExport() error {
	plan := p.Plan

	err := os.MkdirAll(filepath.Join(p.Dir, DataDir), 0770)
	if err != nil {
		return err
	}

	exportCommand := bmongo.MongoExportCommand{
		Conn: plan.MongoConnection,
		DB: bmongo.Database{
			Name:        plan.FromTenant,
			Collections: getDBCollections(p.Dir),
		},
		Log:     p.log,
		DataDir: filepath.Join(p.Dir, DataDir),
	}

	p.log.Info("Exporting data...")
	err = exportCommand.Execute()
	if err != nil {
		return errors.Wrap(err, "execute export")
	}

	p.log.Info("Migrating exported data...")
	err = p.migrate()
	if err != nil {
		return errors.Wrap(err, "migrate")
	}

	color.Green("\nExport completed.\n")

	return nil
}

func (p *Planner) migrate() error {

	collections := getDBCollections(p.Dir)
	jobsPath := collections[JobsCollection].DataFile
	connectionsPath := collections[ConnectionsCollection].DataFile
	shapesPath := collections[ShapesCollection].DataFile
	schemasPath := collections[SchemasCollection].DataFile
	writebacksPath := collections[WritebacksCollection].DataFile

	/* UPDATE AGENT IDS   */
	agentMap := p.Plan.AgentMapping

	// in jobs:
	err := p.replaceInFile("update agent IDs", jobsPath, regexp.MustCompile(`"agentId":\s?"[^"]+"`), func(s string) string {
		matches := valueExtractorRE.FindStringSubmatch(s)
		mappedAgent := agentMap[matches[1]]
		return fmt.Sprintf(`"agentId": "%s"`, mappedAgent)
	})
	if err != nil {
		return err
	}

	// in connections:
	err = p.replaceInFile("update agent IDs", connectionsPath, regexp.MustCompile(`"preferredAgent":\s?"[^"]+"`), func(s string) string {
		matches := valueExtractorRE.FindStringSubmatch(s)
		mappedAgent := agentMap[matches[1]]
		return fmt.Sprintf(`"preferredAgent": "%s"`, mappedAgent)
	})
	if err != nil {
		return errors.Wrapf(err, "updating agents in %q", connectionsPath)
	}

	/* DELETE SECRETS */
	err = p.replaceInFile("delete secrets", connectionsPath, regexp.MustCompile(`"vault:tenant-secrets[^"]+"`), func(s string) string {
		return `""`
	})
	if err != nil {
		return err
	}

	/* Pause all jobs */
	err = p.replaceInFile("pause jobs", jobsPath, regexp.MustCompile(`"isPaused":\s?false,`), func(s string) string {
		return `"isPaused": true,`
	})
	if err != nil {
		return err
	}
	err = p.replaceInFile("pause jobs", writebacksPath, regexp.MustCompile(`"status": "[^"]+"`), func(s string) string {
		return `"status": "Paused"`
	})
	if err != nil {
		return err
	}

	/* GIVE ALL JOBS NEW IDS */
	jobIDMap := map[string]string{}

	err = p.replaceInFile("regenerate job IDs", jobsPath, regexp.MustCompile(`"_id":\s?"[^"]+"`), func(s string) string {
		matches := valueExtractorRE.FindStringSubmatch(s)
		oldJobID := matches[1]
		newJobID := xid.New().String()
		jobIDMap[oldJobID] = newJobID
		return fmt.Sprintf(`"_id": "%s"`, newJobID)
	})
	if err != nil {
		return err
	}

	jobIDRE := regexp.MustCompile(strings.Join(util.SortedKeys(jobIDMap), "|"))

	// Update all references to the old job IDs in all resources:
	for _, filePath := range []string{jobsPath, connectionsPath, schemasPath, shapesPath} {
		err = p.replaceInFile("update job ID references", filePath, jobIDRE, func(oldJobID string) string {
			newJobID := jobIDMap[oldJobID]
			if newJobID == "" {
				return fmt.Sprintf("CLONE_INVALID:ORIGINAL=%s", oldJobID)
			}
			return newJobID
		})
		if err != nil {
			return err
		}
	}

	return nil
}

var valueExtractorRE = regexp.MustCompile(`"[^"]+":\s?(?:"([^"]+)"|(null))`)

func (p *Planner) replaceInFile(purpose, path string, re *regexp.Regexp, replacer func(string) string) error {

	b, err := ioutil.ReadFile(path)
	if err != nil {
		return errors.Wrapf(err, "%s: read file", purpose)
	}

	b = re.ReplaceAllFunc(b, func(bytes []byte) []byte {
		replacement := replacer(string(bytes))
		p.log.Debugf("Replacement in %q: %s -> %s", path, string(bytes), replacement)
		return []byte(replacement)
	})

	err = ioutil.WriteFile(path, b, 0600)
	if err != nil {
		return errors.Wrapf(err, "%s: write file", purpose)
	}
	return err
}

func (p *Planner) PerformImport() error {
	plan := p.Plan

	cmd := bmongo.MongoImportCommand{
		Conn: plan.MongoConnection,
		DB: bmongo.Database{
			Name:        plan.ToTenant,
			Collections: getDBCollections(p.Dir),
		},
		Log:     p.log,
		DataDir: filepath.Join(p.Dir, DataDir),
	}

	err := cmd.Execute()
	if err != nil {
		return err
	}

	color.Green("\nImport completed.\n")

	return nil
}

const JobsCollection = "metabase.jobs"
const ConnectionsCollection = "metabase.connections"
const ShapesCollection = "metabase.shapes"
const SchemasCollection = "metabase.schemas"
const WritebacksCollection = "sync.writebacks"

func getDBCollections(dir string) map[string]*bmongo.CollectionInfo {
	collectionNames := []string{
		JobsCollection,
		ConnectionsCollection,
		ShapesCollection,
		SchemasCollection,
		WritebacksCollection,
	}
	out := map[string]*bmongo.CollectionInfo{}
	for _, name := range collectionNames {
		out[name] = &bmongo.CollectionInfo{
			DataFile: filepath.Join(dir, DataDir, fmt.Sprintf("%s.json", name)),
			Drop:     true,
		}
	}

	return out
}

type validationErrs []string

func (v validationErrs) With(msg string, args ...interface{}) validationErrs {
	return append(v, fmt.Sprintf(msg, args...))
}

func getAgents(tenant string, db *mongo.Database) ([]Agent, error) {
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	collection := db.Collection("agents")
	cur, err := collection.Find(ctx, bson.M{"tenantID": tenant})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []Agent
	for cur.Next(ctx) {
		var result bson.M
		err = cur.Decode(&result)
		if err != nil {
			return nil, err
		}
		out = append(out, Agent{
			Name: result["name"].(string),
			ID:   result["_id"].(string),
		})
	}
	return out, nil
}
