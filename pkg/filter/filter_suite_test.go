package filter_test

import (
	. "github.com/naveego/bosun/pkg/filter"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestFilter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Filter Suite")
}

type Item struct {
	Name   string
	Labels map[string]string
}

func (i Item) GetLabels() Labels {
	return LabelsFromMap(i.Labels)
}
