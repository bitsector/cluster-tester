package example_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
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

var _ = ginkgo.Describe("StatefulSet PDB E2E test", ginkgo.Ordered, func() {
	var clientset *kubernetes.Clientset
	var hpaMaxReplicas int32
	var minBDPAllowedPods int32

	ginkgo.BeforeAll(func() {
		fmt.Printf("\n=== Starting StatefulSet PDB E2E test ===\n")

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
	})

	ginkgo.AfterEach(func() {
		clientset.CoreV1().RESTClient().(*rest.RESTClient).Client.CloseIdleConnections()
	})

	ginkgo.AfterAll(func() {
		fmt.Printf("\n=== Final namespace cleanup ===\n")
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
				fmt.Printf("\nError: could not destroy 'test-ns' namespace after 1 minute\n")
				break
			}

			// Handle transient errors
			if err != nil {
				fmt.Printf("Temporary error checking namespace: %v\n", err)
			}

			time.Sleep(interval)
		}
	})

	ginkgo.It("should apply PDB manifests", func() {
		hpaYAML, pdbYAML, ssYAML, err := example.GetPDBStSTestFiles()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Parse HPA YAML to extract maxReplicas
		type hpaSpec struct {
			Spec struct {
				MaxReplicas int32 `yaml:"maxReplicas"`
			} `yaml:"spec"`
		}

		// Parse PDB YAML to extract minBDPAllowedPods
		var hpaConfig hpaSpec
		err = yaml.Unmarshal([]byte(hpaYAML), &hpaConfig)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		hpaMaxReplicas = hpaConfig.Spec.MaxReplicas

		type pdbSpec struct {
			Spec struct {
				MinAvailable int32 `yaml:"minAvailable"`
			} `yaml:"spec"`
		}

		var pdbConfig pdbSpec
		err = yaml.Unmarshal([]byte(pdbYAML), &pdbConfig)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		minBDPAllowedPods = pdbConfig.Spec.MinAvailable
		fmt.Printf("\n=== Minimum allowed pods from PDB: %d ===\n", minBDPAllowedPods)

		// Apply all the manifests
		fmt.Printf("\n=== Applying StatefulSet and Service manifest ===\n")
		err = example.ApplyRawManifest(clientset, ssYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Applying PDB manifest ===\n")
		err = example.ApplyRawManifest(clientset, pdbYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// HPA is probably not needed here
		fmt.Printf("\n=== Applying HPA manifest (maxReplicas: %d) ===\n", hpaMaxReplicas)
		err = example.ApplyRawManifest(clientset, hpaYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Wait for Pods to schedule ===\n")
		time.Sleep(30 * time.Second)
	})

	ginkgo.It("should maintain minimum pod count during deletions", func() {
		//Get current pod count
		pods, err := clientset.CoreV1().Pods("test-ns").List(
			context.TODO(),
			metav1.ListOptions{FieldSelector: "status.phase=Running"},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		initialPods := len(pods.Items)
		fmt.Printf("\n=== Initial running pods: %d ===\n", initialPods)

		// Verify minimum pod count
		gomega.Expect(int32(initialPods)).To(
			gomega.BeNumerically(">=", minBDPAllowedPods),
			fmt.Sprintf("Initial pods (%d) below PDB minimum (%d)", initialPods, minBDPAllowedPods),
		)

		// Delete all pods
		fmt.Printf("\n=== Deleting all %d pods ===\n", initialPods)
		for _, pod := range pods.Items {
			err := clientset.CoreV1().Pods("test-ns").Delete(
				context.TODO(),
				pod.Name,
				metav1.DeleteOptions{},
			)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}

		// Immediate post-deletion checks with 5 attempts
		fmt.Printf("\n=== Performing post-deletion validation (several attempts) ===\n")
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

			fmt.Printf("Attempt %d: Running Pods=%d, Sampling Duration=%v\n",
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

		fmt.Printf("\n=== All post-deletion checks passed ===\n")
	})

})
