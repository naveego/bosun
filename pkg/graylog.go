package pkg

import (
	"encoding/json"
	"github.com/dghubble/sling"
	"github.com/naveego/bosun/pkg/core"
	"github.com/pkg/errors"
	"net/http"
	"net/http/httputil"
)

type GraylogConfig struct {
	Username  string
	Password  string
	ApiOrigin string `yaml:"apiOrigin" json:"apiOrigin"`
	Inputs    []GraylogInput
	DryRun    bool `yaml:"dryRun" json:"dryRun"`
	client    *sling.Sling
}

type GraylogInput struct {
	Title         string                 `json:"title"`
	Type          string                 `json:"type"`
	Global        bool                   `json:"global"`
	Configuration map[string]interface{} `json:"configuration"`
	Node          string                 `json:"node"`
}

func (g *GraylogConfig) Apply() error {

	g.client = sling.New().Base(g.ApiOrigin).SetBasicAuth(g.Username, g.Password)

	err := g.applyInputs()
	if err != nil {
		return err
	}

	return nil
}

func (g *GraylogConfig) req() (*sling.Sling) {
	return g.client.New()
}

func (g *GraylogConfig) call(call *sling.Sling) (map[string]interface{}, error) {
	var data map[string]interface{}
	req, _ := call.Request()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if resp.StatusCode >= 300 {
		respMsg, _ := httputil.DumpResponse(resp, true)
		reqMsg, _ := httputil.DumpRequestOut(req, true)

		return nil, errors.Errorf("Error.\nRequest:\n%s\n\nResponse:\n%s", string(reqMsg), string(respMsg))
	}

	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&data)

	if err != nil {
		msg, _ := httputil.DumpResponse(resp, true)
		reqMsg, _ := httputil.DumpRequestOut(req, true)
		return nil, errors.Errorf("error decoding response: %s\nresponse was:\n%s\n\nrequest was:\n%s\n", err, string(msg), string(reqMsg))
	}

	return data, err
}

func (g *GraylogConfig) applyInputs() error {

	var data map[string]interface{}
	data, err := g.call(g.req().Get("/api/system/inputs"))
	if err != nil {
		return err
	}

	existingInputs := data["inputs"].([]interface{})
	titles := map[string]string{}
	for _, i := range existingInputs {
		input := i.(map[string]interface{})
		titles[input["title"].(string)] = input["id"].(string)
	}

	core.Log.WithField("count", len(g.Inputs)).Debug("Processing inputs.")

	for _, input := range g.Inputs {

		if id, ok := titles[input.Title]; ok {
			core.Log.WithField("title", input.Title).Debug("Updating input.")
			_, err := g.call(g.req().Put("/api/system/inputs/" + id).BodyJSON(input))
			if err != nil {
				return err
			}
		} else {
			core.Log.WithField("title", input.Title).Debug("Creating input.")
			_, err := g.call(g.req().Post("/api/system/inputs").BodyJSON(input))
			if err != nil {
				return err
			}
		}
	}

	core.Log.Debug("Inputs processed.")

	return nil

}
