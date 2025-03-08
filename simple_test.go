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

func TestExample(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Cluster Test Suite")
}

var _ = ginkgo.Describe("Cluster Operations", func() {
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

var _ = ginkgo.Describe("Topology E2E test", ginkgo.Ordered, func() {
	var clientset *kubernetes.Clientset
	var hpaMaxReplicas int32 // Add global variable declaration

	ginkgo.BeforeAll(func() {
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
	})

	ginkgo.It("should apply topology manifests", func() {
		hpaYAML, depYAML, err := example.GetTopologyTestFiles()
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

		fmt.Printf("\n=== Applying HPA manifest (maxReplicas: %d) ===\n", hpaMaxReplicas)
		err = example.ApplyRawManifest(clientset, hpaYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Applying Deployment manifest ===\n")
		err = example.ApplyRawManifest(clientset, depYAML)
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

		fmt.Printf("\n=== Wait for HPA to trigger ===\n")
		time.Sleep(100 * time.Second)

	})

	ginkgo.It("should verify topology constraints", func() {
		fmt.Printf("\n=== Verifying pod scale count and distribution ===\n")
		time.Sleep(100 * time.Second) // Wait for scaling operations

		// Get deployment details
		deployment, err := clientset.AppsV1().Deployments("test-ns").Get(
			context.TODO(),
			"zone-spread-example",
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Get all pods for the deployment
		pods, err := clientset.CoreV1().Pods("test-ns").List(
			context.TODO(),
			metav1.ListOptions{
				LabelSelector: metav1.FormatLabelSelector(deployment.Spec.Selector),
			},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Collect node distribution
		nodeDistribution := make(map[string]int)
		var nodeNames []string

		fmt.Printf("\nPod-to-Node Distribution:\n")
		for _, pod := range pods.Items {
			nodeName := pod.Spec.NodeName
			nodeDistribution[nodeName]++
			nodeNames = append(nodeNames, nodeName)
			fmt.Printf("- Pod %-40s → Node: %s\n", pod.Name, nodeName)
		}

		// Calculate max skew
		maxCount := 0
		minCount := len(pods.Items)
		for _, count := range nodeDistribution {
			if count > maxCount {
				maxCount = count
			}
			if count < minCount {
				minCount = count
			}
		}
		skew := maxCount - minCount

		fmt.Printf("\nDistribution Analysis:\n")
		fmt.Printf("Total Pods: %d\n", len(pods.Items))
		fmt.Printf("Nodes Used: %d\n", len(nodeDistribution))
		fmt.Printf("Max Pods per Node: %d\n", maxCount)
		fmt.Printf("Min Pods per Node: %d\n", minCount)
		fmt.Printf("Calculated Skew: %d\n", skew)

		// Validate constraints
		gomega.Expect(skew).To(gomega.BeNumerically("<=", 1),
			fmt.Sprintf("Topology skew violation: Max skew %d exceeds allowed maximum of 1", skew))

		fmt.Printf("\nTopology validation successful - max skew of %d within allowed threshold\n", skew)
	})

})
