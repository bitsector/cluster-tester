package example_test

import (
	"context"
	"example"
	"fmt"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/rs/zerolog"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestConnectivity(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Basic cluster connectivity test")
}

var _ = ginkgo.Describe("Basic cluster connectivity test", ginkgo.Ordered, ginkgo.Label("safe-in-production"), func() {
	var (
		clientset *kubernetes.Clientset
		logger    zerolog.Logger
		testTag   = "SimpleConnectivityTest"
	)

	ginkgo.BeforeAll(func() {
		var err error
		clientset, err = example.GetClient()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		logger = example.GetLogger(testTag)

		// Namespace setup
		logger.Info().Msgf("=== Creating test-ns namespace ===")
		_, err = clientset.CoreV1().Namespaces().Create(
			context.TODO(),
			&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}},
			metav1.CreateOptions{},
		)
		if apierrors.IsAlreadyExists(err) {
			logger.Info().Msgf("Namespace test-ns already exists\n")
		} else {
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			logger.Info().Msgf("Namespace test-ns created successfully\n")
		}

		// Register cleanup inside setup node
		ginkgo.DeferCleanup(func() {
			logger.Info().Msgf("=== Final namespace cleanup ===")
			err := clientset.CoreV1().Namespaces().Delete(
				context.TODO(),
				"test-ns",
				metav1.DeleteOptions{},
			)
			if err != nil && !apierrors.IsNotFound(err) {
				ginkgo.Fail(fmt.Sprintf("Final cleanup failed: %v", err))
			}

			// Verification loop
			const (
				timeout  = 1 * time.Minute
				interval = 500 * time.Millisecond
			)
			deadline := time.Now().Add(timeout)

			for {
				_, err := clientset.CoreV1().Namespaces().Get(
					context.TODO(),
					"test-ns",
					metav1.GetOptions{},
				)

				if apierrors.IsNotFound(err) {
					logger.Info().Msgf("Namespace test-ns successfully removed\n")
					break
				}

				if time.Now().After(deadline) {
					logger.Info().Msgf("\nError: Namespace test-ns still exists after 1 minute\n")
					break
				}

				if err != nil {
					logger.Info().Msgf("Transient error verifying deletion: %v\n", err)
				}

				time.Sleep(interval)
			}

			clientset.CoreV1().RESTClient().(*rest.RESTClient).Client.CloseIdleConnections()
		})
	})

	ginkgo.It("should list cluster nodes", func() {
		defer example.E2ePanicHandler()

		logger.Info().Msgf("=== Listing cluster nodes ===")
		nodes, err := clientset.CoreV1().Nodes().List(
			context.TODO(),
			metav1.ListOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(nodes.Items).NotTo(gomega.BeEmpty())

		logger.Info().Msgf("Discovered %d nodes:\n", len(nodes.Items))
		for i, node := range nodes.Items {
			logger.Info().Msgf("%d. %s\n", i+1, node.Name)
		}
	})

	ginkgo.It("should have ready nodes", func() {
		defer example.E2ePanicHandler()

		nodes, err := clientset.CoreV1().Nodes().List(
			context.TODO(),
			metav1.ListOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		logger.Info().Msgf("=== Checking node readiness ===")
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
			logger.Info().Msgf("Node %-30s: %s\n", node.Name, status)
		}
	})

	ginkgo.It("should have test namespace", func() {
		defer example.E2ePanicHandler()

		logger.Info().Msgf("=== Verifying test namespace ===")
		_, err := clientset.CoreV1().Namespaces().Get(
			context.TODO(),
			"test-ns",
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		logger.Info().Msgf("Namespace test-ns verified\n")
	})
})
