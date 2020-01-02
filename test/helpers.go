package test

import (
	"github.com/naveego/bosun/pkg/bosun"
	"github.com/naveego/bosun/pkg/cli"
	. "github.com/onsi/gomega"
)

func NewTestInstance() *bosun.Bosun {
	return NewTestInstanceParams(cli.Parameters{})
}

func NewTestInstanceParams(params cli.Parameters) *bosun.Bosun {
	ws, err := bosun.LoadWorkspace("./.bosun/bosun.yaml")
	Expect(err).ToNot(HaveOccurred())

	b, err := bosun.New(params, ws)
	Expect(err).ToNot(HaveOccurred())
	return b

}
