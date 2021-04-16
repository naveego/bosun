// +build !windows

package command_test

import (
	"context"
	"fmt"
	"github.com/naveego/bosun/pkg/command"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("ShellExe", func() {

	It("should execute in director", func() {
		sut := command.NewShellExe("pwd").WithDir("/tmp")
		Expect(sut.RunOut()).To(Equal("/tmp"))
	})

	It("should respect context cancel", func() {
		ctx, cancel := context.WithCancel(context.Background())
		sut := command.NewShellExe("sleep", "1000").WithContext(ctx)
		errCh := make(chan error)
		go func() {
			errCh <- sut.RunE()
		}()
		<-time.After(10 * time.Millisecond)
		cancel()
		var expected error
		Eventually(errCh, 1000*time.Millisecond).Should(Receive(&expected))
		fmt.Println(expected)
	})
})
