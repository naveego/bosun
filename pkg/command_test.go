// +build !windows

package pkg_test

import (
	"context"
	"fmt"
	"github.com/naveego/bosun/pkg"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("Command", func() {

	It("should execute in director", func(){
		sut := pkg.NewCommand("pwd").WithDir("/tmp")
		Expect(sut.RunOut()).To(Equal("/tmp"))
	})

	It("should respect context cancel", func(){
		ctx, cancel := context.WithCancel(context.Background())
		sut := pkg.NewCommand("sleep", "1000").WithContext(ctx)
		errCh := make(chan error)
		go func(){
			errCh <- sut.RunE()
		}()
		<-time.After(10*time.Millisecond)
		cancel()
		var expected error
		Eventually(errCh, 1000*time.Millisecond).Should(Receive(&expected))
		fmt.Println(expected)
	})
})
