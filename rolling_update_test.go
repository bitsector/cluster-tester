package example_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
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
		depStartYAML   []byte
		hpaYAML        []byte
		newDeployment  *appsv1.Deployment
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
		depStartYAML, hpaYAML, _, err = example.GetRollingUpdateTestFiles()
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

	})

	ginkgo.It("should perform rolling update with updated CPU requests", func() {
		fmt.Printf("\n=== Triggering rolling update with new CPU requests ===\n")

		currentDeployment, err := clientset.AppsV1().Deployments("test-ns").Get(
			context.TODO(),
			"app",
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// FIX 1: Remove colon to assign to shared variable
		newDeployment = currentDeployment.DeepCopy()
		newDeployment.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceCPU] = resource.MustParse("100m")

		// FIX 2: Capture updated deployment from API response
		updatedDeployment, err := clientset.AppsV1().Deployments("test-ns").Update(
			context.TODO(),
			newDeployment,
			metav1.UpdateOptions{
				FieldManager: "e2e-test",
			},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		newDeployment = updatedDeployment

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

		// FIX 3: Refresh deployment status after rollout
		finalDeployment, err := clientset.AppsV1().Deployments("test-ns").Get(
			context.TODO(),
			"app",
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		newDeployment = finalDeployment
	})
})
