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

func TestStatefulSetTopology(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "StatefulSet Topology Constraints Suite")
}

var _ = ginkgo.Describe("StatefulSet Topology Constraints E2E test", ginkgo.Ordered, ginkgo.Label("safe-in-production"), func() {
	var (
		clientset      *kubernetes.Clientset
		hpaMaxReplicas int32
		logger         zerolog.Logger
		testTag        = "StatefulSetTopologyConstraitTest"
	)

	ginkgo.BeforeAll(func() {
		logger.Info().Msgf("=== Starting StatefulSet Topology Constraints E2E test ===")

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

	ginkgo.It("should apply topology manifests", func() {
		defer example.E2ePanicHandler()

		hpaYAML, ssYAML, err := example.GetStatefulSetTestFiles()
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

		logger.Info().Msgf("=== Applying StatefulSet and Service manifest ===")
		err = example.ApplyRawManifest(clientset, ssYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		logger.Info().Msgf("=== Applying HPA manifest (maxReplicas: %d) ===", hpaMaxReplicas)
		err = example.ApplyRawManifest(clientset, hpaYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		time.Sleep(10 * time.Second)
	})

	ginkgo.It("should verify topology resources exist", func() {
		defer example.E2ePanicHandler()

		logger.Info().Msgf("=== Verifying cluster resources ===")

		// Check StatefulSet exists
		statefulSets, err := clientset.AppsV1().StatefulSets("test-ns").List(
			context.TODO(),
			metav1.ListOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(statefulSets.Items).NotTo(gomega.BeEmpty())
		logger.Info().Msgf("Found %d statefulSets in namespace:\n", len(statefulSets.Items))
		for _, d := range statefulSets.Items {
			logger.Info().Msgf("- %s (Replicas: %d)\n", d.Name, *d.Spec.Replicas)
		}

		// Check HPA exists
		hpas, err := clientset.AutoscalingV2().HorizontalPodAutoscalers("test-ns").List(
			context.TODO(),
			metav1.ListOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(hpas.Items).NotTo(gomega.BeEmpty())
		logger.Info().Msgf("Found %d HPAs in namespace:\n", len(hpas.Items))
		for _, h := range hpas.Items {
			logger.Info().Msgf("- %s (Min: %d, Max: %d)\n",
				h.Name,
				*h.Spec.MinReplicas,
				h.Spec.MaxReplicas,
			)
		}

		logger.Info().Msgf("=== Wait for HPA to trigger scaling ===")
		deadline := time.Now().Add(5 * time.Minute)
		pollInterval := 5 * time.Second

		for {
			// Get current pod count for StatefulSet
			currentPods, err := clientset.CoreV1().Pods("test-ns").List(
				context.TODO(),
				metav1.ListOptions{
					LabelSelector: "app=myapp",
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

	ginkgo.It("should verify topology constraints", func() {
		defer example.E2ePanicHandler()

		logger.Info().Msgf("=== Verifying pod scale count and distribution ===")

		statefulSet, err := clientset.AppsV1().StatefulSets("test-ns").Get(
			context.TODO(),
			"zone-spread-example",
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		pods, err := clientset.CoreV1().Pods("test-ns").List(
			context.TODO(),
			metav1.ListOptions{
				LabelSelector: metav1.FormatLabelSelector(statefulSet.Spec.Selector),
			},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Get unique node names from all pods
		nodeNames := make(map[string]struct{})
		for _, pod := range pods.Items {
			if pod.Spec.NodeName != "" {
				nodeNames[pod.Spec.NodeName] = struct{}{}
			}
		}

		// Build node-to-zone mapping
		nodeToZone := make(map[string]string)
		for nodeName := range nodeNames {
			node, err := clientset.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			zone, ok := node.Labels["topology.kubernetes.io/zone"]
			if !ok {
				ginkgo.Fail(fmt.Sprintf("Node %s missing zone label", nodeName))
			}
			nodeToZone[nodeName] = zone
		}

		// Collect zone distribution
		zoneDistribution := make(map[string]int)
		logger.Info().Msgf("\nPod-to-Zone Distribution:\n")
		for _, pod := range pods.Items {
			zone := nodeToZone[pod.Spec.NodeName]
			zoneDistribution[zone]++
			logger.Info().Msgf("- Pod %-40s → Zone: %s\n", pod.Name, zone)
		}

		// Calculate max skew between zones
		maxCount := 0
		minCount := len(pods.Items)
		for _, count := range zoneDistribution {
			if count > maxCount {
				maxCount = count
			}
			if count < minCount {
				minCount = count
			}
		}
		skew := maxCount - minCount

		logger.Info().Msgf("\nZone Distribution Analysis:\n")
		logger.Info().Msgf("Total Pods: %d\n", len(pods.Items))
		logger.Info().Msgf("Zones Used: %d\n", len(zoneDistribution))
		logger.Info().Msgf("Max Pods per Zone: %d\n", maxCount)
		logger.Info().Msgf("Min Pods per Zone: %d\n", minCount)
		logger.Info().Msgf("Calculated Skew: %d\n", skew)

		gomega.Expect(skew).To(gomega.BeNumerically("<=", 1),
			fmt.Sprintf("Topology skew violation: Max zone skew %d exceeds allowed maximum of 1", skew))

		logger.Info().Msgf("\nZone topology validation successful - max skew of %d within threshold\n", skew)
	})

})
