package featureworkflow_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestFeatureworkflow(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Featureworkflow Suite")
}
