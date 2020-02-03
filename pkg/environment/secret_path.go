package environment

import (
	"github.com/pkg/errors"
	"net/url"
	"strconv"
	"strings"
)

type SecretPath struct {
	GroupName  string
	SecretName string
	Generation *SecretGenerationConfig
}

func ParseSecretPath(in string) (*SecretPath, error) {

	u, err := url.Parse(in)
	if err != nil {
		return nil, err
	}

	segs := strings.Split(u.Path, "/")
	if len(segs) != 2 {
		return nil, errors.Errorf("invalid secret path %q (want group/path?params)", in)
	}

	out := &SecretPath{
		GroupName:  segs[0],
		SecretName: segs[1],
	}

	for key, value := range u.Query() {
		if len(value) == 1 {
			switch key {
			case "length":
				if out.Generation == nil {
					out.Generation = &SecretGenerationConfig{}
				}
				out.Generation.Length, err = strconv.Atoi(value[0])
			default:
				return nil, errors.Errorf("unrecognized secret path parameter %q in %q", key, in)
			}
		} else {
			return nil, errors.Errorf("invalid secret path parameter %q in %q", key, in)
		}
	}

	return out, nil

}
