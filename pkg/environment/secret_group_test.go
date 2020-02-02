package environment_test

import (
	. "github.com/naveego/bosun/pkg/environment"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"os"
	"path/filepath"
)

var pwd, _ = os.Getwd()
var environmentPath = filepath.Join(pwd, "__testdata__/environment.yaml")


var _ = Describe("Secret", func() {

	const secretGroup = "default"
	const secretName = "example"
	const secretValue = "example-value"

	It("can generate passwords", func(){
		for i := 0; i < 10; i++ {
			password := SecureRandomPassword(DefaultPasswordAlphabet, 20)
			Expect(password).To(HaveLen(20))
		}
	})

	It("can load and save secrets", func(){

		environment := loadTestEnvironmentWithSecret(secretGroup, secretName, secretValue)
		actual, err := environment.GetSecretValue(secretGroup, secretName)
		Expect(err).ToNot(HaveOccurred())
		Expect(actual).To(Equal(secretValue))
	})

	It("can delete secret values", func(){
		environment := loadTestEnvironmentWithSecret(secretGroup, secretName, secretValue)
		group, found, err := environment.GetSecretGroup(secretGroup)
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(group.DeleteSecretValue(secretName)).To(Succeed())
		environment = loadTestEnvironment()
		_, err = environment.GetSecretValue(secretGroup, secretName)
		Expect(err).To(MatchError(ContainSubstring("found uninitialized secret but it did not have generation settings")))
	})

	It("can delete secrets", func(){
		environment := loadTestEnvironmentWithSecret(secretGroup, secretName, secretValue)
		group, found, err := environment.GetSecretGroup(secretGroup)
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(group.DeleteSecretConfig(secretName)).To(Succeed())
		environment = loadTestEnvironment()
		_, err = environment.GetSecretValue(secretGroup, secretName)
		Expect(err).To(MatchError(ContainSubstring("group did not contain secret")))
	})

	It("can delete group", func(){
		environment := loadTestEnvironmentWithSecret(secretGroup, secretName, secretValue)
		Expect(environment.DeleteSecretGroup(secretGroup)).To(Succeed())
		environment = loadTestEnvironment()
		_, found, err := environment.GetSecretGroup(secretGroup)
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeFalse())
	})

	It("can generate and persist secret", func(){
		const generateSecretName = "generate"
		environment := loadTestEnvironment()
		Expect(environment.AddSecretGroup(secretGroup, SecretKeyConfig{UnsafeStoredPassphrase: "test"})).To(Succeed())
		group, found, err := environment.GetSecretGroup(secretGroup)
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())

		expectedPasswordLength := 10
		err = group.AddOrUpdateSecret("", SecretConfig{
			Name:generateSecretName,
			Generation: &SecretGenerationConfig {
				Length: expectedPasswordLength,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		environment = loadTestEnvironment()
		actual, err := environment.GetSecretValue(secretGroup, generateSecretName)
		Expect(err).ToNot(HaveOccurred())
		Expect(actual).To(HaveLen(expectedPasswordLength))
	})

	It("can validate secrets", func(){
		const generateSecretName = "generate"
		environment := loadTestEnvironmentWithSecret(secretGroup, secretName, secretValue)
		group, found, err := environment.GetSecretGroup(secretGroup)
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		expectedPasswordLength := 10
		err = group.AddOrUpdateSecret("", SecretConfig{
			Name:generateSecretName,
			Generation: &SecretGenerationConfig {
				Length: expectedPasswordLength,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		environment = loadTestEnvironment()
		Expect(environment.ValidateSecrets(secretGroup+"/"+secretName, secretGroup+"/"+generateSecretName)).To(Succeed())
	})
})

func loadTestEnvironmentWithSecret(secretGroup, secretName, secretValue string) *Environment {
	environment := loadTestEnvironment()
	Expect(environment.AddSecretGroup(secretGroup, SecretKeyConfig{UnsafeStoredPassphrase: "test"})).To(Succeed())
	Expect(environment.AddOrUpdateSecretValue(secretGroup, secretName, secretValue)).To(Succeed())
	Expect(environment.Save()).To(Succeed())

	environment = loadTestEnvironment()

	return environment
}

func loadTestEnvironment() *Environment {
	environmentConfig, err := LoadConfig(environmentPath)
	Expect(err).ToNot(HaveOccurred())
	environment, err := New(*environmentConfig, Options{})
	Expect(err).ToNot(HaveOccurred())

	return environment
}