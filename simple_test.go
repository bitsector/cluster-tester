package example_test

import (
	"context"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"example"
)

func TestExample(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Cluster Test Suite")
}

var _ = ginkgo.Describe("Basic Tests", func() {
	ginkgo.It("should pass basic math", func() {
		gomega.Expect(1 + 1).To(gomega.Equal(2))
	})

	ginkgo.Describe("Cluster Operations", func() {
		var clientset *kubernetes.Clientset

		ginkgo.BeforeEach(func() {
			var err error
			clientset, err = example.GetClient()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Add cleanup using Ginkgo's built-in mechanism
			ginkgo.DeferCleanup(func() {
				clientset.CoreV1().RESTClient().(*rest.RESTClient).Client.CloseIdleConnections()
			})
		})

		ginkgo.It("should list cluster nodes", func() {
			nodes, err := clientset.CoreV1().Nodes().List(
				context.TODO(),
				metav1.ListOptions{},
			)

			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(nodes.Items).NotTo(gomega.BeEmpty())
		})

		ginkgo.It("should have ready nodes", func() {
			nodes, err := clientset.CoreV1().Nodes().List(
				context.TODO(),
				metav1.ListOptions{},
			)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			for _, node := range nodes.Items {
				ready := false
				for _, cond := range node.Status.Conditions {
					if cond.Type == v1.NodeReady && cond.Status == v1.ConditionTrue {
						ready = true
						break
					}
				}
				gomega.Expect(ready).To(gomega.BeTrue(), "Node %s should be ready", node.Name)
			}
		})
	})
})
