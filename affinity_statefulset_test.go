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

func TestStatefulSetAffinity(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "StatefulSet Affinity Test Suite")
}

var _ = ginkgo.Describe("StatefulSet Affinity E2E test", ginkgo.Ordered, func() {
	var clientset *kubernetes.Clientset
	var hpaMaxReplicas int32

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

	ginkgo.It("should apply affinity manifests", func() {
		hpaYAML, zoneYAML, ssYAML, err := example.GetAffinityStatefulSetTestFiles()
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

		fmt.Printf("\n=== Applying Affinity StatefulSet and Service manifest ===\n")
		err = example.ApplyRawManifest(clientset, ssYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Applying HPA manifest (maxReplicas: %d) ===\n", hpaMaxReplicas)
		err = example.ApplyRawManifest(clientset, hpaYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Wait for HPA to be triggered ===\n")
		time.Sleep(90 * time.Second)
	})

	ginkgo.It("should ensure dependent pods are in same zone as zone-marker", func() {
		// Get zone-marker pod details using correct label selector
		fmt.Printf("\n=== Getting zone-marker pod details ===\n")
		markerPods, err := clientset.CoreV1().Pods("test-ns").List(
			context.TODO(),
			metav1.ListOptions{
				LabelSelector: "app=desired-zone-for-affinity", // Updated to match YAML labels
			},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(markerPods.Items).To(gomega.HaveLen(1),
			"Should have exactly one zone-marker pod. Check deployment labels.")

		markerPod := markerPods.Items[0]
		markerNode, err := clientset.CoreV1().Nodes().Get(
			context.TODO(),
			markerPod.Spec.NodeName,
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		markerZone := markerNode.Labels["topology.kubernetes.io/zone"]
		fmt.Printf("Zone-Marker Pod: %s\nNode: %s\nZone: %s\n",
			markerPod.Name, markerPod.Spec.NodeName, markerZone)

		// Get dependent-app pods details
		fmt.Printf("\n=== Getting dependent-app pods details ===\n")
		ssPods, err := clientset.CoreV1().Pods("test-ns").List(
			context.TODO(),
			metav1.ListOptions{
				LabelSelector: "app=dependent-app",
			},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(ssPods.Items).NotTo(gomega.BeEmpty(),
			"No dependent-app pods found. Check StatefulSet status.")

		var depZones []string
		for _, pod := range ssPods.Items {
			node, err := clientset.CoreV1().Nodes().Get(
				context.TODO(),
				pod.Spec.NodeName,
				metav1.GetOptions{},
			)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			zone := node.Labels["topology.kubernetes.io/zone"]
			depZones = append(depZones, zone)
			fmt.Printf("Dependent Pod: %s\nNode: %s\nZone: %s\n",
				pod.Name, pod.Spec.NodeName, zone)
		}

		// Validate all zones match
		fmt.Printf("\n=== Validating zone consistency ===\n")
		fmt.Printf("Zone-Marker Zone: %s\nDependent Pod Zones: %v\n", markerZone, depZones)
		gomega.Expect(depZones).To(gomega.HaveEach(markerZone),
			"All dependent pods should be in the same zone as zone-marker")
	})

})
