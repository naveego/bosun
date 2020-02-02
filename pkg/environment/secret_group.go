package environment

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"github.com/naveego/bosun/pkg/core"
	"github.com/naveego/bosun/pkg/yaml"
	"github.com/pkg/errors"
	"strings"
)

type SecretGroupConfig struct {
	core.ConfigShared `yaml:",inline"`
	Secrets           []*SecretConfig  `yaml:"secrets"`
	Key               *SecretKeyConfig `yaml:"key"`

	SecretValues string `yaml:"secretValues"`

	// true if this is a new secret group that has no saved file
	isNew bool `yaml:"-"`
}

type SecretGroup struct {
	config *SecretGroupConfig

	valuesDirty bool

	// contains the values of secrets that have been loaded or added
	values map[string]string `yaml:"-" json:"-"`
}

// Creates an in-memory secret group from the given config, with secret values decrypted.
func NewSecretGroup(config *SecretGroupConfig) (*SecretGroup, error) {
	group, err := newSecretGroup(config)
	if err != nil {
		return nil, errors.Wrapf(err, "load secrets for %s", config.Name)
	}
	return group, nil
}

func newSecretGroup(s *SecretGroupConfig) (*SecretGroup, error) {

	group := &SecretGroup{
		config: s,
		values: map[string]string{},
	}

	if s.isNew {
		return group, nil
	}

	key, nonce, err := group.config.Key.GetKeyComponents(s.Name)
	if err != nil {
		return nil, err
	}

	hextext := s.SecretValues
	if len(hextext) == 0 {
		return group, nil
	}

	ciphertext, err := hex.DecodeString(strings.Replace(string(hextext), "\n", "", -1))
	if err != nil {
		return nil, errors.Wrap(err, "invalid secrets file (should be hex encoded)")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err.Error())
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err.Error())
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.Wrap(err, "invalid secret data")
	}

	var values map[string]string
	err = yaml.Unmarshal(plaintext, &values)
	if err != nil {
		return nil, errors.Wrapf(err, "")
	}

	group.values = values
	return group, nil
}

func (s *SecretGroup) setValue(name string, value string) {
	if s.values == nil {
		s.values = map[string]string{}
	}
	if existing, ok := s.values[name]; ok && existing == value {
		return
	}
	s.valuesDirty = true
	s.values[name] = value
}

func (s *SecretGroup) deleteValue(name string) {
	if s.values == nil {
		s.values = map[string]string{}
	}
	if _, ok := s.values[name]; ok {
		delete(s.values, name)
		s.valuesDirty = true
		return
	}
}

func (s *SecretGroup) GetSecretValue(name string) (string, error) {
	secret, err := s.getSecretValue(name)
	if err != nil {
		return "", errors.Wrapf(err, "get secret %q from group %q", name, s.config.Name)
	}
	return secret, nil
}

func (s *SecretGroup) getSecretValue(name string) (string, error) {
	if value, ok := s.values[name]; ok {
		return value, nil
	}

	var secretConfig *SecretConfig
	for _, sc := range s.config.Secrets {
		if sc.Name == name {
			secretConfig = sc
		}
	}
	if secretConfig == nil {
		return "", errors.New("group did not contain secret")
	}

	if secretConfig.Generation == nil {
		return "", errors.New("found uninitialized secret but it did not have generation settings")
	}

	if secretConfig.Generation.Length == 0 {
		secretConfig.Generation.Length = 20
	}

	password := SecureRandomPassword(DefaultPasswordAlphabet, secretConfig.Generation.Length)

	s.setValue(name, password)

	err := s.Save()
	if err != nil {
		return "", errors.Wrap(err, "saving newly generated secret")
	}

	return password, err

}

// AddOrUpdateSecretValue adds or replaces an existing secret, then saves the group.
func (s *SecretGroup) AddOrUpdateSecret(value string, config SecretConfig) error {
	if config.Name == "" {
		return errors.New("secret name is required")
	}

	if value == "" {
		if config.Generation == nil {
			return errors.New("if value is not provided then generation config must not be nil")
		}
	} else {
		s.setValue(config.Name, value)
	}

	replaced := false
	for i, existing := range s.config.Secrets {
		if existing.Name == config.Name {
			s.config.Secrets[i] = &config
			replaced = true
			break
		}
	}
	if !replaced {
		s.config.Secrets = append(s.config.Secrets, &config)
	}

	s.valuesDirty = true
	return s.Save()
}

func (s *SecretGroup) AddOrUpdateSecretValue(name string, value string) error {
	if name == "" {
		return errors.New("secret name is required")
	}
	if value == "" {
		return errors.New("secret value is required")
	}

	found := false
	for _, existing := range s.config.Secrets {
		if existing.Name == name {
			found = true
			break
		}
	}
	if !found {
		s.config.Secrets = append(s.config.Secrets, &SecretConfig{
			Name:name,
		})
	}

	s.setValue(name, value)

	return s.Save()
}

// DeleteSecretConfig deletes a secret config (and the value) from the group, then saves the group.
func (s *SecretGroup) DeleteSecretConfig(name string) error {
	var secrets []*SecretConfig
	for _, secret := range s.config.Secrets {
		if secret.Name != name {
			secrets = append(secrets, secret)
		}
	}

	s.config.Secrets = secrets

	s.deleteValue(name)

	return s.Save()
}

// DeleteSecret deletes the value of a secret (but not the config) from the group, then saves the group.
func (s *SecretGroup) DeleteSecretValue(name string) error {
	s.deleteValue(name)
	return s.Save()
}

// DeleteAllSecretValues drops all secret values, then saves the group.
func (s *SecretGroup) DeleteAllSecretValues() error {
	s.values = map[string]string{}
	s.valuesDirty = true
	return s.Save()
}

// Save encrypts the secret values and saves them to disk.
func (s *SecretGroup) Save() error {
	err := s.save()
	return errors.Wrapf(err, "save secret group %s", s.config.Name)
}

func (s *SecretGroup) save() error {

	if s.valuesDirty {
		// discard previous nonce because we are encrypting new info
		s.config.Key.Nonce = ""

		if len(s.values) == 0 {
			s.config.SecretValues = ""
		} else {
			key, nonce, err := s.config.Key.GetKeyComponents(s.config.Name)
			if err != nil {
				return err
			}

			block, err := aes.NewCipher(key)
			if err != nil {
				panic(err.Error())
			}

			aesgcm, err := cipher.NewGCM(block)
			if err != nil {
				panic(err.Error())
			}

			plaintext, _ := yaml.Marshal(s.values)

			ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)

			s.config.SecretValues = hex.EncodeToString(ciphertext)
		}
	}

	err := yaml.SaveYaml(s.config.FromPath, s.config)

	return err
}

const (
	DefaultPasswordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
)

func SecureRandomPassword(alphabet string, length int) string {

	// Compute bitMask
	availableCharLength := len(alphabet)
	if availableCharLength == 0 || availableCharLength > 256 {
		panic("availableCharBytes length must be greater than 0 and less than or equal to 256")
	}
	var bitLength byte
	var bitMask byte
	for bits := availableCharLength - 1; bits != 0; {
		bits = bits >> 1
		bitLength++
	}
	bitMask = 1<<bitLength - 1
	result := make([]byte, length)
	bufferSize := int(float64(length) * 1.3)
	for i, j, randomBytes := 0, 0, []byte{}; i < length; j++ {
		if j%bufferSize == 0 {
			randomBytes = secureRandomBytes(bufferSize)
		}
		if idx := int(randomBytes[j%length] & bitMask); idx < availableCharLength {
			result[i] = alphabet[idx]
			i++
		}
	}

	return string(result)
}

// SecureRandomBytes returns the requested number of bytes using crypto/rand
func secureRandomBytes(length int) []byte {
	var randomBytes = make([]byte, length)
	_, err := rand.Read(randomBytes)
	if err != nil {
		panic("crypto rand is broken")
	}
	return randomBytes
}
