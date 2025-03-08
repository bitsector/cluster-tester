package example_test

import (
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestExample(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Example Suite")
}

var _ = ginkgo.Describe("Basic Test", func() {
	ginkgo.It("should pass", func() {
		gomega.Expect(1 + 1).To(gomega.Equal(2))
	})
})
