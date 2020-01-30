package environment_test

import (
	"github.com/naveego/bosun/integration/testhelpers"
	. "github.com/naveego/bosun/pkg/environment"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"os"
	"path/filepath"
)

var pwd, _ = os.Getwd()
var environmentPath = filepath.Join(pwd, "__testdata__/environment.yaml")


var _ = Describe("Secret", func() {



	It("can load and save secrets", func(){

		environment := loadTestEnvironment()

		const secretGroup = "default"
		const secretName = "example"
		const secretValue = "example-value"

		Expect(environment.AddOrReKeySecretGroup(secretGroup, `echo "test"`)).To(Succeed())
		Expect(environment.AddOrUpdateSecret(secretGroup, &Secret{
			SecretConfig: SecretConfig{
				Name:secretName,
			},
			Value: secretValue,
		})).To(Succeed())

		Expect(environment.Save()).To(Succeed())

		environment = loadTestEnvironment()

		actual, err := environment.GetSecretValue(secretGroup, secretName)
		Expect(err).ToNot(HaveOccurred())
		Expect(actual).To(Equal(secretValue))
	})
})

func loadTestEnvironment() *Environment {
	ctx := testhelpers.NewMockExecutionContext()
	environmentConfig, err := LoadConfig(environmentPath)
	Expect(err).ToNot(HaveOccurred())
	environment, err := New(*environmentConfig, Options{})
	Expect(err).ToNot(HaveOccurred())

	Expect(environment.PrepareSecrets(ctx)).To(Succeed())
	return environment
}