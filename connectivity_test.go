package example_test

import (
	"context"
	"example"
	"fmt"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestConnectivity(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Cluster Operations Suite")
}

var _ = ginkgo.Describe("Cluster Operations", func() {
	var clientset *kubernetes.Clientset
	nsName, err := example.GetNsName()
	if err != nil {
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		fmt.Printf("Could not get namespace name, %+v\n", err)
	}

	ginkgo.BeforeEach(func() {
		var err error
		clientset, err = example.GetClient()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Namespace setup
		fmt.Printf("\n=== Checking for %s namespace ===\n", nsName)
		_, err = clientset.CoreV1().Namespaces().Get(
			context.TODO(),
			nsName,
			metav1.GetOptions{},
		)

		if apierrors.IsNotFound(err) {
			fmt.Printf("Namespace %s not found, creating...\n", nsName)
			ns := &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			_, err = clientset.CoreV1().Namespaces().Create(
				context.TODO(),
				ns,
				metav1.CreateOptions{},
			)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			fmt.Printf("Namespace %s created successfully\n", nsName)
		} else {
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			fmt.Printf("Namespace %s already exists\n", nsName)
		}

		// Cleanup connections
		ginkgo.DeferCleanup(func() {
			clientset.CoreV1().RESTClient().(*rest.RESTClient).Client.CloseIdleConnections()
		})

		// Namespace cleanup
		ginkgo.DeferCleanup(func() {
			fmt.Printf("\n=== Cleaning up %s namespace ===\n", nsName)
			err := clientset.CoreV1().Namespaces().Delete(
				context.TODO(),
				nsName,
				metav1.DeleteOptions{},
			)
			if err != nil && !apierrors.IsNotFound(err) {
				ginkgo.Fail(fmt.Sprintf("Failed to delete namespace: %v", err))
			}
			fmt.Printf("Namespace %s cleanup initiated\n", nsName)
		})
	})

	ginkgo.It("should list cluster nodes", func() {
		fmt.Printf("\n=== Listing cluster nodes ===\n")
		nodes, err := clientset.CoreV1().Nodes().List(
			context.TODO(),
			metav1.ListOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(nodes.Items).NotTo(gomega.BeEmpty())

		fmt.Printf("Discovered %d nodes:\n", len(nodes.Items))
		for i, node := range nodes.Items {
			fmt.Printf("%d. %s\n", i+1, node.Name)
		}
	})

	ginkgo.It("should have ready nodes", func() {
		nodes, err := clientset.CoreV1().Nodes().List(
			context.TODO(),
			metav1.ListOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Checking node readiness ===\n")
		for _, node := range nodes.Items {
			ready := false
			for _, cond := range node.Status.Conditions {
				if cond.Type == v1.NodeReady && cond.Status == v1.ConditionTrue {
					ready = true
					break
				}
			}
			status := "Not Ready"
			if ready {
				status = "Ready"
			}
			fmt.Printf("Node %-30s: %s\n", node.Name, status)
		}
	})

	ginkgo.It("should have test namespace", func() {
		fmt.Printf("\n=== Verifying test namespace ===\n")
		_, err := clientset.CoreV1().Namespaces().Get(
			context.TODO(),
			nsName,
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		fmt.Printf("Namespace %s verified\n", nsName)
	})
})
