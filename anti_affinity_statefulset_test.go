package example_test

import (
	"context"
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

var _ = ginkgo.Describe("StatefulSet Anti Affinity E2E test", ginkgo.Ordered, ginkgo.Label("safe-in-production"), func() {
	var (
		clientset      *kubernetes.Clientset
		hpaMaxReplicas int32
		logger         zerolog.Logger
		testTag        = "StatefulSetAntiAffinityTest"
	)

	ginkgo.BeforeAll(func() {

		var err error
		clientset, err = example.GetClient()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		logger = example.GetLogger(testTag)

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
		if ginkgo.CurrentSpecReport().Failed() {
			logger.Error().Msgf("%s:TEST_FAILED", testTag)
		}

	})

	ginkgo.AfterAll(func() {
		example.ClearNamespace(logger, clientset)
	})

	ginkgo.It("should apply anti affinity manifests", func() {
		logger.Info().Msgf("=== Starting StatefulSet Anti Affinity E2E test ===")
		logger.Info().Msgf("=== tag: %s, allowed to fail: %t", testTag, example.IsTestAllowedToFail(testTag))

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

		logger.Info().Msgf("=== Applying Zone Marker manifest ===")
		err = example.ApplyRawManifest(clientset, zoneYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		logger.Info().Msgf("=== Applying Anti Affinity StatefulSet and Service manifest ===")
		err = example.ApplyRawManifest(clientset, ssYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		logger.Info().Msgf("=== Applying HPA manifest (maxReplicas: %d) ===", hpaMaxReplicas)
		err = example.ApplyRawManifest(clientset, hpaYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		logger.Info().Msgf("=== Wait for HPA to trigger scaling ===")
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
			logger.Info().Msgf("Waiting for HPA, Current running pods: %d/%d\n", runningCount, hpaMaxReplicas)

			if runningCount >= int(hpaMaxReplicas) {
				logger.Info().Msgf("Waiting for HPA, Reached required pod count of %d\n", hpaMaxReplicas)
				break
			}

			if time.Now().After(deadline) {
				ginkgo.Fail("Failed to wait for the HPA to get to the maximum required pods")
			}

			time.Sleep(pollInterval)
		}
	})

	ginkgo.It("should enforce zone separation between zone-marker and dependent-app", func() {

		// Get zone-marker pod information
		logger.Info().Msgf("=== Getting zone-marker pod details ===")
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
			logger.Info().Msgf("Zone-Marker Pod: %-20s Node: %-15s Zone: %s\n",
				detail.name, detail.node, detail.zone)
		}

		// Get dependent-app pods
		logger.Info().Msgf("=== Getting dependent-app pods details ===")
		dependentPods, err := clientset.CoreV1().Pods("test-ns").List(
			context.TODO(),
			metav1.ListOptions{LabelSelector: "app=dependent-app"},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(dependentPods.Items).NotTo(gomega.BeEmpty(), "No dependent-app pods found")

		// Verify zone separation
		logger.Info().Msgf("=== Validating zone constraints ===")
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

			logger.Info().Msgf("Dependent Pod: %-20s Node: %-15s Zone: %s\n",
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

		logger.Info().Msgf("Zone-Marker Zones (forbidden): %v\nDependent Pod Zones: %v\n",
			forbiddenZonesSlice, dependentAppZones)
	})

})
