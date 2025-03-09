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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"example"
)

func TestRollingUpdate(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Affinity Test Suite")
}

var _ = ginkgo.Describe("Rolling Update E2E test", ginkgo.Ordered, func() {
	var (
		clientset      *kubernetes.Clientset
		hpaMaxReplicas int32
		depEndYAML     []byte
		depStartYAML   []byte
		hpaYAML        []byte
	)

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

	ginkgo.It("should apply Rolling update manifests", func() {
		var err error
		depStartYAML, hpaYAML, depEndYAML, err = example.GetRollingUpdateTestFiles()
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

		// Apply all the manifests
		fmt.Printf("\n=== Applying Initial deployment manifest (maxReplicas: %d) ===\n", hpaMaxReplicas)
		err = example.ApplyRawManifest(clientset, depStartYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// HPA will probably not be used here
		fmt.Printf("\n=== Applying HPA manifest ===\n")
		err = example.ApplyRawManifest(clientset, hpaYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// fmt.Printf("\n=== Applying Deployment manifest ===\n")
		// err = example.ApplyRawManifest(clientset, depEndYAML)
		// gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== %d ===\n", len(depEndYAML))

		fmt.Printf("\n=== Wait for Pods to chedule ===\n")
		time.Sleep(30 * time.Second)
	})

	ginkgo.It("should perform rolling update with updated CPU requests", func() {
		fmt.Printf("\n=== Triggering rolling update with new CPU requests ===\n")

		// Get existing deployment
		currentDeployment, err := clientset.AppsV1().Deployments("test-ns").Get(
			context.TODO(),
			"app",
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Update container spec using server-side apply
		newDeployment := currentDeployment.DeepCopy()
		newDeployment.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceCPU] = resource.MustParse("100m")

		_, err = clientset.AppsV1().Deployments("test-ns").Update(
			context.TODO(),
			newDeployment,
			metav1.UpdateOptions{
				FieldManager: "e2e-test",
			},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Waiting for rollout to complete ===\n")
		gomega.Eventually(func() bool {
			deployment, err := clientset.AppsV1().Deployments("test-ns").Get(
				context.TODO(),
				"app",
				metav1.GetOptions{},
			)
			if err != nil {
				return false
			}
			return deployment.Status.UpdatedReplicas == *deployment.Spec.Replicas &&
				deployment.Status.Replicas == *deployment.Spec.Replicas &&
				deployment.Status.AvailableReplicas == *deployment.Spec.Replicas
		}, 3*time.Minute, 5*time.Second).Should(gomega.BeTrue())

		fmt.Printf("\n=== Verifying pod resource updates ===\n")
		pods, err := clientset.CoreV1().Pods("test-ns").List(
			context.TODO(),
			metav1.ListOptions{
				LabelSelector: "app=app",
			},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Verify all pods are using new CPU requests
		for _, pod := range pods.Items {
			ginkgo.By(fmt.Sprintf("Checking pod %s", pod.Name))

			// Ensure pod is running and ready
			gomega.Expect(pod.Status.Phase).To(gomega.Equal(v1.PodRunning))
			gomega.Expect(pod.Status.ContainerStatuses[0].Ready).To(gomega.BeTrue())

			// Verify resource requests
			container := pod.Spec.Containers[0]
			cpuRequest := container.Resources.Requests[v1.ResourceCPU]
			memoryRequest := container.Resources.Requests[v1.ResourceMemory]

			gomega.Expect(cpuRequest.String()).To(gomega.Equal("100m"),
				"Pod %s has incorrect CPU request", pod.Name)
			gomega.Expect(memoryRequest.String()).To(gomega.Equal("64Mi"),
				"Pod %s has incorrect Memory request", pod.Name)

			// Additional verification for rolling update behavior
			ginkgo.By(fmt.Sprintf("Verify pod creation timestamp > deployment update time"))
			gomega.Expect(pod.CreationTimestamp.After(newDeployment.CreationTimestamp.Time)).To(gomega.BeTrue(),
				"Pod %s was not created after deployment update", pod.Name)
		}
	})
})
