package actions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"github.com/hashicorp/go-getter/helper/url"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"strings"
)

type HTTPAction struct {
	URL     string                 `yaml:"url" json:"url"`
	Method  string                 `yaml:"method" json:"method"`
	Headers map[string]string      `yaml:"headers" json:"headers"`
	Body    map[string]interface{} `yaml:"body,omitempty" json:"body"`
	Raw     string                 `yaml:"raw,omitempty" json:"raw,omitempty"`
	OKCodes []int                  `yaml:"okCodes,omitempty,flow" json:"okCodes,omitempty"` // codes which should be treated as OK (passing). Defaults to [200, 201, 202, 204].
}

func (a *HTTPAction) Execute(ctx ActionContext) error {

	var req *http.Request
	var err error
	if a.Raw == "" {
		var bodyBytes []byte
		var err error
		if a.Body != nil {
			bodyBytes, err = json.Marshal(a.Body)
			if err != nil {
				return errors.Wrap(err, "marshal body")
			}
		}

		bodyBuffer := bytes.NewBuffer(bodyBytes)
		a.Method = strings.ToUpper(a.Method)

		ctx.Log().Debugf("Making %s request to %s...", a.Method, a.URL)

		req, err = http.NewRequest(a.Method, a.URL, bodyBuffer)
		if err != nil {
			return errors.Wrap(err, "create req")
		}
		for k, v := range a.Headers {
			req.Header.Add(k, v)
		}
	} else {
		if !strings.Contains(a.Raw, "\n\n") {
			a.Raw = a.Raw + "\n\n"
		}

		r := bufio.NewReader(strings.NewReader(a.Raw))

		req, err = http.ReadRequest(r)
		if err != nil {
			return errors.Wrapf(err, "create req from raw input:\n%s", a.Raw)
		}
		req.URL, err = url.Parse(req.RequestURI)
		if err != nil {
			return errors.Wrapf(err, "parse url %q", req.RequestURI)
		}
		req.RequestURI = ""
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "made request")
	}

	ctx.Log().Debugf("Request returned %d - %s.", resp.StatusCode, resp.Status)

	if len(a.OKCodes) == 0 {
		a.OKCodes = []int{http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent}
	}

	defer resp.Body.Close()
	isOK := false
	for _, okCode := range a.OKCodes {
		if resp.StatusCode == okCode {
			isOK = true
			break
		}
	}

	if !isOK {
		respBody, _ := ioutil.ReadAll(resp.Body)
		err = errors.Errorf("Response had non-success status code %d (OKCodes: %v): %s", resp.StatusCode, a.OKCodes, string(respBody))
		return err
	}

	return nil
}
