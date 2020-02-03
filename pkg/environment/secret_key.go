package environment

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/cli"
	"github.com/pkg/errors"
	"golang.org/x/crypto/scrypt"
	"os"
)

type SecretKeyConfig struct {
	Prompt                 bool   `yaml:"prompt,omitempty"`
	EnvironmentVariable    string `yaml:"environmentVariable,omitempty"`
	UnsafeStoredPassphrase string `yaml:"UNSAFE,omitempty"`
	Lastpass              *LastpassKeyConfig `yaml:"lastpass,omitempty"`
	Nonce               string `yaml:"nonce"`
	Salt                string `yaml:"salt"`

	key []byte `yaml:"-"`
}
type LastpassKeyConfig  struct {
	Path  string `yaml:"path"`
	Field string `yaml:"field"`
}

func (s *SecretKeyConfig) GetKeyComponents(secretGroupName string) (key []byte, nonce []byte, err error) {


	if s.Salt == "" {
		b := make([]byte, 16)
		_, err = rand.Read(b)
		if err != nil {
			panic(err)
		}
		s.Salt = hex.EncodeToString(b)
	}
	salt, err := hex.DecodeString(s.Salt)
	if err != nil {
		return nil, nil, errors.Wrap(err, "invalid salt")
	}

	if s.Nonce == "" {
		b := make([]byte, 12)
		_, err = rand.Read(b)
		if err != nil {
			panic(err)
		}
		s.Nonce = fmt.Sprintf("%x", b)
	}
	nonce, err = hex.DecodeString(s.Nonce)
	if err != nil {
		return nil, nil, errors.Wrap(err, "invalid nonce")
	}

	if s.key == nil {

		passphrase := s.UnsafeStoredPassphrase
		if passphrase == "" {
			passphrase = os.Getenv(s.EnvironmentVariable)
		}
		if passphrase == "" && s.Prompt {
			passphrase = cli.RequestSecretFromUser("Provide passphrase for secret group %s", secretGroupName)
		}
		if passphrase == "" && s.Lastpass != nil {
			if s.Lastpass.Field == "" {
				s.Lastpass.Field = "password"
			}

			passphrase, err = pkg.NewShellExe("lpass", "show", s.Lastpass.Path, "--field", s.Lastpass.Field).RunOut()
			if err != nil {
				return nil, nil, errors.Wrapf(err, "lastpass lookup failed (used path %q and field %q)", s.Lastpass.Path, s.Lastpass.Field)
			}
		}
		if passphrase == "" {
			return nil, nil, errors.Errorf("could not get passphrase from secret file, environment variable %q, or lastpass", s.EnvironmentVariable)
		}

		s.key, err = scrypt.Key([]byte(passphrase), salt, 16384, 8, 1, 32)
		if err != nil {
			panic(err)
		}
	}

	return s.key, nonce, nil
}
