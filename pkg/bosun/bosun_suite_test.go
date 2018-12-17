package bosun_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestBosun(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bosun Suite")
}
