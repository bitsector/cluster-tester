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

func TestStatefulSetAntiAffinity(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "StatefulSet Anti Affinity Test Suite")
}

var _ = ginkgo.Describe("StatefulSet Anti Affinity E2E test", ginkgo.Ordered, ginkgo.Label("safe-in-production"), func() {
	var clientset *kubernetes.Clientset
	var hpaMaxReplicas int32
	var logger zerolog.Logger

	ginkgo.BeforeAll(func() {
		fmt.Printf("\n=== Starting StatefulSet Anti Affinity E2E test ===\n")

		var err error
		clientset, err = example.GetClient()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		logger = example.GetLogger("StatefulSetAntiAffinityTest")
		logger.Info().Msg("StateflSet Anti Affinity Test zerolog init")

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

	ginkgo.It("should apply anti affinity manifests", func() {
		defer example.E2ePanicHandler()

		hpaYAML, zoneYAML, ssYAML, err := example.GetAntiAffinityStatefulSetTestFiles()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Parse HPA YAML to extract maxReplicas
		type hpaSpec struct {
			Spec struct {
				MaxReplicas int32 `yaml:"maxReplicas"`
			} `yaml:"spec"`
		}

		var hpaConfig hpaSpec
		err = yaml.Unmarshal([]byte(hpaYAML), &hpaConfig)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		hpaMaxReplicas = hpaConfig.Spec.MaxReplicas

		fmt.Printf("\n=== Applying Zone Marker manifest ===\n")
		err = example.ApplyRawManifest(clientset, zoneYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Applying Anti Affinity StatefulSet and Service manifest ===\n")
		err = example.ApplyRawManifest(clientset, ssYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Applying HPA manifest (maxReplicas: %d) ===\n", hpaMaxReplicas)
		err = example.ApplyRawManifest(clientset, hpaYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Wait for HPA to trigger scaling ===\n")
		deadline := time.Now().Add(5 * time.Minute)
		pollInterval := 5 * time.Second

		for {
			// Get current pod count for StatefulSet
			currentPods, err := clientset.CoreV1().Pods("test-ns").List(
				context.TODO(),
				metav1.ListOptions{
					LabelSelector: "app=dependent-app",
					FieldSelector: "status.phase=Running",
				},
			)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			runningCount := len(currentPods.Items)
			fmt.Printf("Waiting for HPA, Current running pods: %d/%d\n", runningCount, hpaMaxReplicas)

			if runningCount >= int(hpaMaxReplicas) {
				fmt.Printf("Waiting for HPA, Reached required pod count of %d\n", hpaMaxReplicas)
				break
			}

			if time.Now().After(deadline) {
				ginkgo.Fail("Failed to wait for the HPA to get to the maximum required pods")
			}

			time.Sleep(pollInterval)
		}
	})

	ginkgo.It("should enforce zone separation between zone-marker and dependent-app", func() {
		defer example.E2ePanicHandler()

		// Get zone-marker pod information
		fmt.Printf("\n=== Getting zone-marker pod details ===\n")
		zoneMarkerPods, err := clientset.CoreV1().Pods("test-ns").List(
			context.TODO(),
			metav1.ListOptions{LabelSelector: "app=desired-zone-for-anti-affinity"},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(zoneMarkerPods.Items).NotTo(gomega.BeEmpty(), "No zone-marker pods found")

		// Collect unique forbidden zones using a map
		forbiddenZones := make(map[string]struct{})
		type podInfo struct{ name, node, zone string }
		var zoneMarkerDetails []podInfo

		for _, zmPod := range zoneMarkerPods.Items {
			node, err := clientset.CoreV1().Nodes().Get(
				context.TODO(),
				zmPod.Spec.NodeName,
				metav1.GetOptions{},
			)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			zone := node.Labels["topology.kubernetes.io/zone"]
			gomega.Expect(zone).NotTo(gomega.BeEmpty(),
				"Zone label missing on node %s", zmPod.Spec.NodeName)

			forbiddenZones[zone] = struct{}{}
			zoneMarkerDetails = append(zoneMarkerDetails, podInfo{
				name: zmPod.Name,
				node: zmPod.Spec.NodeName,
				zone: zone,
			})
		}

		// Log zone-marker details
		for _, detail := range zoneMarkerDetails {
			fmt.Printf("Zone-Marker Pod: %-20s Node: %-15s Zone: %s\n",
				detail.name, detail.node, detail.zone)
		}

		// Get dependent-app pods
		fmt.Printf("\n=== Getting dependent-app pods details ===\n")
		dependentPods, err := clientset.CoreV1().Pods("test-ns").List(
			context.TODO(),
			metav1.ListOptions{LabelSelector: "app=dependent-app"},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(dependentPods.Items).NotTo(gomega.BeEmpty(), "No dependent-app pods found")

		// Verify zone separation
		fmt.Printf("\n=== Validating zone constraints ===\n")
		var dependentAppZones []string
		for _, depPod := range dependentPods.Items {
			node, err := clientset.CoreV1().Nodes().Get(
				context.TODO(),
				depPod.Spec.NodeName,
				metav1.GetOptions{},
			)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			podZone := node.Labels["topology.kubernetes.io/zone"]
			gomega.Expect(podZone).NotTo(gomega.BeEmpty(),
				"Zone label missing on node %s", depPod.Spec.NodeName)

			fmt.Printf("Dependent Pod: %-20s Node: %-15s Zone: %s\n",
				depPod.Name, depPod.Spec.NodeName, podZone)

			dependentAppZones = append(dependentAppZones, podZone)

			// Check against deduplicated forbidden zones
			gomega.Expect(forbiddenZones).NotTo(gomega.HaveKey(podZone),
				"Pod %s in prohibited zone %s", depPod.Name, podZone)
		}

		// Convert forbidden zones map to slice for logging
		forbiddenZonesSlice := make([]string, 0, len(forbiddenZones))
		for zone := range forbiddenZones {
			forbiddenZonesSlice = append(forbiddenZonesSlice, zone)
		}

		fmt.Printf("Zone-Marker Zones (forbidden): %v\nDependent Pod Zones: %v\n",
			forbiddenZonesSlice, dependentAppZones)
	})

})
