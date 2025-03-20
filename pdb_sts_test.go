package example_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"example"
)

func TestStatefulSetPDB(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "StatefulSet PDB Test Suite")
}

var _ = ginkgo.Describe("StatefulSet PDB E2E test", ginkgo.Ordered, ginkgo.Label("safe-in-production"), func() {
	var clientset *kubernetes.Clientset
	var minBDPAllowedPods int32
	var logger zerolog.Logger

	ginkgo.BeforeAll(func() {
		logger.Info().Msgf("=== Starting StatefulSet PDB E2E test ===")

		var err error
		clientset, err = example.GetClient()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		logger = example.GetLogger("StatefulSetPDBTest")

		// Namespace setup
		logger.Info().Msgf("=== Ensuring test-ns exists ===")
		_, err = clientset.CoreV1().Namespaces().Get(
			context.TODO(),
			"test-ns",
			metav1.GetOptions{},
		)

		if apierrors.IsNotFound(err) {
			logger.Info().Msgf("Creating test-ns namespace\n")
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
	})

	ginkgo.AfterEach(func() {
		clientset.CoreV1().RESTClient().(*rest.RESTClient).Client.CloseIdleConnections()
	})

	ginkgo.AfterAll(func() {
		logger.Info().Msgf("=== Final namespace cleanup ===")
		err := clientset.CoreV1().Namespaces().Delete(
			context.TODO(),
			"test-ns",
			metav1.DeleteOptions{},
		)
		if err != nil && !apierrors.IsNotFound(err) {
			ginkgo.Fail(fmt.Sprintf("Final cleanup failed: %v", err))
		}

		// Namespace existence verification loop
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
				break // Namespace successfully deleted
			}

			if time.Now().After(deadline) {
				logger.Info().Msgf("\nError: could not destroy 'test-ns' namespace after 1 minute\n")
				break
			}

			// Handle transient errors
			if err != nil {
				logger.Info().Msgf("Temporary error checking namespace: %v\n", err)
			}

			time.Sleep(interval)
		}
	})

	ginkgo.It("should apply PDB manifests", func() {
		defer example.E2ePanicHandler()

		pdbYAML, ssYAML, err := example.GetPDBStSTestFiles()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		type pdbSpec struct {
			Spec struct {
				MinAvailable int32 `yaml:"minAvailable"`
			} `yaml:"spec"`
		}

		var pdbConfig pdbSpec
		err = yaml.Unmarshal([]byte(pdbYAML), &pdbConfig)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		minBDPAllowedPods = pdbConfig.Spec.MinAvailable
		logger.Info().Msgf("=== Minimum allowed pods from PDB: %d ===", minBDPAllowedPods)

		// Apply all the manifests
		logger.Info().Msgf("=== Applying StatefulSet and Service manifest ===")
		err = example.ApplyRawManifest(clientset, ssYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		logger.Info().Msgf("=== Applying PDB manifest ===")
		err = example.ApplyRawManifest(clientset, pdbYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		logger.Info().Msgf("=== Wait for Pods to schedule ===")
		time.Sleep(30 * time.Second)
	})

	ginkgo.It("should maintain minimum pod count during deletions", func() {
		defer example.E2ePanicHandler()

		//Get current pod count
		pods, err := clientset.CoreV1().Pods("test-ns").List(
			context.TODO(),
			metav1.ListOptions{FieldSelector: "status.phase=Running"},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		initialPods := len(pods.Items)
		logger.Info().Msgf("=== Initial running pods: %d ===", initialPods)

		// Verify minimum pod count
		gomega.Expect(int32(initialPods)).To(
			gomega.BeNumerically(">=", minBDPAllowedPods),
			fmt.Sprintf("Initial pods (%d) below PDB minimum (%d)", initialPods, minBDPAllowedPods),
		)

		// Delete all pods
		logger.Info().Msgf("=== Deleting all %d pods ===", initialPods)
		for _, pod := range pods.Items {
			err := clientset.CoreV1().Pods("test-ns").Delete(
				context.TODO(),
				pod.Name,
				metav1.DeleteOptions{},
			)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}

		// Immediate post-deletion checks with 5 attempts
		logger.Info().Msgf("=== Performing post-deletion validation (several attempts) ===")
		numAttempts := 10
		for attempt := 1; attempt <= numAttempts; attempt++ {
			startPostCheck := time.Now()
			postDeletePods, err := clientset.CoreV1().Pods("test-ns").List(
				context.TODO(),
				metav1.ListOptions{FieldSelector: "status.phase=Running"},
			)
			postCheckDuration := time.Since(startPostCheck)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			finalPods := len(postDeletePods.Items)

			logger.Info().Msgf("Attempt %d: Running Pods=%d, Sampling Duration=%v\n",
				attempt,
				finalPods,
				postCheckDuration.Round(time.Millisecond))

			gomega.Expect(int32(finalPods)).To(
				gomega.BeNumerically(">=", minBDPAllowedPods),
				fmt.Sprintf("Attempt %d: Running Pod count (%d) violated PDB minimum (%d)",
					attempt,
					finalPods,
					minBDPAllowedPods),
			)
		}

		logger.Info().Msgf("=== All post-deletion checks passed ===")
	})

})
