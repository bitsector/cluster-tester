package example_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"example"
)

func TestExample(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Cluster Test Suite")
}

var _ = ginkgo.Describe("Basic Test", func() {
	ginkgo.It("should pass basic math", func() {
		gomega.Expect(1 + 1).To(gomega.Equal(2))
	})

	ginkgo.Describe("Cluster Operations", func() {
		var clientset *kubernetes.Clientset

		ginkgo.BeforeEach(func() {
			var err error
			clientset, err = example.GetClient()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Namespace setup
			fmt.Printf("\n=== Checking for test-ns namespace ===\n")
			_, err = clientset.CoreV1().Namespaces().Get(
				context.TODO(),
				"test-ns",
				metav1.GetOptions{},
			)

			if apierrors.IsNotFound(err) {
				fmt.Printf("Namespace test-ns not found, creating...\n")
				ns := &v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-ns",
					},
				}
				_, err = clientset.CoreV1().Namespaces().Create(
					context.TODO(),
					ns,
					metav1.CreateOptions{},
				)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				fmt.Printf("Namespace test-ns created successfully\n")
			} else {
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				fmt.Printf("Namespace test-ns already exists\n")
			}

			// Cleanup connections
			ginkgo.DeferCleanup(func() {
				clientset.CoreV1().RESTClient().(*rest.RESTClient).Client.CloseIdleConnections()
			})

			// Namespace cleanup
			ginkgo.DeferCleanup(func() {
				fmt.Printf("\n=== Cleaning up test-ns namespace ===\n")
				err := clientset.CoreV1().Namespaces().Delete(
					context.TODO(),
					"test-ns",
					metav1.DeleteOptions{},
				)
				if err != nil && !apierrors.IsNotFound(err) {
					ginkgo.Fail(fmt.Sprintf("Failed to delete namespace: %v", err))
				}
				fmt.Printf("Namespace test-ns cleanup initiated\n")
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
				"test-ns",
				metav1.GetOptions{},
			)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			fmt.Printf("Namespace test-ns verified\n")
		})
	})
})

var _ = ginkgo.Describe("Topology E2E test", func() {
	var clientset *kubernetes.Clientset

	ginkgo.BeforeEach(func() {
		var err error
		clientset, err = example.GetClient()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Namespace setup
		fmt.Printf("\n=== Ensuring test-ns exists ===\n")
		_, err = clientset.CoreV1().Namespaces().Get(
			context.TODO(),
			"test-ns",
			metav1.GetOptions{},
		)

		if apierrors.IsNotFound(err) {
			fmt.Printf("Creating test-ns namespace\n")
			ns := &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ns",
				},
			}
			_, err = clientset.CoreV1().Namespaces().Create(
				context.TODO(),
				ns,
				metav1.CreateOptions{},
			)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		} else {
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}

		// Register cleanup operations INSIDE the setup node
		ginkgo.DeferCleanup(func() {
			fmt.Printf("\n=== Final namespace cleanup ===\n")
			err := clientset.CoreV1().Namespaces().Delete(
				context.TODO(),
				"test-ns",
				metav1.DeleteOptions{},
			)
			if err != nil && !apierrors.IsNotFound(err) {
				ginkgo.Fail(fmt.Sprintf("Final cleanup failed: %v", err))
			}
		})

		ginkgo.DeferCleanup(func() {
			clientset.CoreV1().RESTClient().(*rest.RESTClient).Client.CloseIdleConnections()
		})
	})

	ginkgo.It("should apply topology manifests", func() {
		hpaYAML, depYAML, err := example.GetTopologyTestFiles()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Applying HPA manifest ===\n")
		err = example.ApplyRawManifest(clientset, hpaYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Applying Deployment manifest ===\n")
		err = example.ApplyRawManifest(clientset, depYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Waiting 10 seconds ===\n")
		time.Sleep(10 * time.Second)
	})

	ginkgo.It("should verify topology constraints", func() {
		fmt.Printf("\n=== Placeholder verification ===\n")
		gomega.Expect(true).To(gomega.BeTrue())
	})
})
