package environment_test

import (
	. "github.com/naveego/bosun/pkg/environment"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)


var _ = Describe("Environment", func() {

	const secretGroup = "default"
	const secretName = "example"
	const secretValue = "example-value"

	It("can get secret using path", func(){
		environment := loadTestEnvironment()
		Expect(environment.AddSecretGroup(secretGroup, &SecretKeyConfig{UnsafeStoredPassphrase:"test"}))

		secret, err := environment.ResolveSecretPath("default/resolved?length=10")
		Expect(err).ToNot(HaveOccurred())

		Expect(secret).To(HaveLen(10))

		environment = loadTestEnvironment()

		loaded, err := environment.GetSecretValue("default", "resolved")
		Expect(err).ToNot(HaveOccurred())
		Expect(loaded).To(Equal(secret))

	})

})
