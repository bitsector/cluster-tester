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

func TestDeploymentTopology(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Deployment Topology Constraints Suite")
}

var _ = ginkgo.Describe("Deployment Topology Constraints E2E test", ginkgo.Ordered, func() {
	var clientset *kubernetes.Clientset
	var hpaMaxReplicas int32 // Add global variable declaration

	ginkgo.BeforeAll(func() {
		fmt.Printf("\n=== Starting Deployment Topology Constraints E2E test ===\n")

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

	ginkgo.It("should apply topology manifests", func() {
		hpaYAML, depYAML, err := example.GetTopologyDeploymentTestFiles()
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

		fmt.Printf("\n=== Applying Deployment manifest ===\n")
		err = example.ApplyRawManifest(clientset, depYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Applying HPA manifest (maxReplicas: %d) ===\n", hpaMaxReplicas)
		err = example.ApplyRawManifest(clientset, hpaYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		time.Sleep(10 * time.Second)
	})

	ginkgo.It("should verify topology resources exist", func() {
		fmt.Printf("\n=== Verifying cluster resources ===\n")

		// Check Deployment exists
		deployments, err := clientset.AppsV1().Deployments("test-ns").List(
			context.TODO(),
			metav1.ListOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(deployments.Items).NotTo(gomega.BeEmpty())
		fmt.Printf("Found %d deployments in namespace:\n", len(deployments.Items))
		for _, d := range deployments.Items {
			fmt.Printf("- %s (Replicas: %d)\n", d.Name, *d.Spec.Replicas)
		}

		// Check HPA exists
		hpas, err := clientset.AutoscalingV2().HorizontalPodAutoscalers("test-ns").List(
			context.TODO(),
			metav1.ListOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(hpas.Items).NotTo(gomega.BeEmpty())
		fmt.Printf("Found %d HPAs in namespace:\n", len(hpas.Items))
		for _, h := range hpas.Items {
			fmt.Printf("- %s (Min: %d, Max: %d)\n",
				h.Name,
				*h.Spec.MinReplicas,
				h.Spec.MaxReplicas,
			)
		}

		fmt.Printf("\n=== Wait for HPA to trigger scaling ===\n")
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

	ginkgo.It("should verify topology constraints", func() {
		fmt.Printf("\n=== Verifying pod scale count and distribution ===\n")

		deployment, err := clientset.AppsV1().Deployments("test-ns").Get(
			context.TODO(),
			"zone-spread-example",
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		pods, err := clientset.CoreV1().Pods("test-ns").List(
			context.TODO(),
			metav1.ListOptions{
				LabelSelector: metav1.FormatLabelSelector(deployment.Spec.Selector),
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
		fmt.Printf("\nPod-to-Zone Distribution:\n")
		for _, pod := range pods.Items {
			zone := nodeToZone[pod.Spec.NodeName]
			zoneDistribution[zone]++
			fmt.Printf("- Pod %-40s → Zone: %s\n", pod.Name, zone)
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

		fmt.Printf("\nZone Distribution Analysis:\n")
		fmt.Printf("Total Pods: %d\n", len(pods.Items))
		fmt.Printf("Zones Used: %d\n", len(zoneDistribution))
		fmt.Printf("Max Pods per Zone: %d\n", maxCount)
		fmt.Printf("Min Pods per Zone: %d\n", minCount)
		fmt.Printf("Calculated Skew: %d\n", skew)

		gomega.Expect(skew).To(gomega.BeNumerically("<=", 1),
			fmt.Sprintf("Topology skew violation: Max zone skew %d exceeds allowed maximum of 1", skew))

		fmt.Printf("\nZone topology validation successful - max skew of %d within threshold\n", skew)
	})

})
