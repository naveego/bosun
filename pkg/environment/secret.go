package environment

type SecretConfig struct {
	Name string `yaml:"name"`
	Description string `yaml:"description"`
	Generation *SecretGenerationConfig `yaml:"generation,omitempty"`
}

type SecretGenerationConfig struct {
	Length int `yaml:"length"`
}
