package test

import (
	"github.com/naveego/bosun/pkg/bosun"
	. "github.com/onsi/gomega"
)

func NewTestInstance() *bosun.Bosun {
	return NewTestInstanceParams(bosun.Parameters{})
}

func NewTestInstanceParams(params bosun.Parameters) *bosun.Bosun {
	ws, err := bosun.LoadWorkspace("./.bosun/bosun.yaml")
	Expect(err).ToNot(HaveOccurred())

	b, err := bosun.New(params, ws)
	Expect(err).ToNot(HaveOccurred())
	return b

}
