package environment

type SecretConfig struct {
	Name string `yaml:"name"`
	Generation *SecretGenerationConfig `yaml:"generation,omitempty"`
}

type SecretGenerationConfig struct {
	Length int `yaml:"length"`
}
