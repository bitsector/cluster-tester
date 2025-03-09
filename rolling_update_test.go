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
	"k8s.io/apimachinery/pkg/util/intstr"
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

		// Retrieve strategy parameters
		deployment, err := clientset.AppsV1().Deployments("test-ns").Get(
			context.TODO(),
			"app",
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		rollingUpdate := deployment.Spec.Strategy.RollingUpdate
		if rollingUpdate == nil {
			ginkgo.Fail("Deployment does not have RollingUpdate strategy")
		}

		replicas := *deployment.Spec.Replicas
		maxSurgeValue, err := intstr.GetValueFromIntOrPercent(rollingUpdate.MaxSurge, int(replicas), true)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		maxUnavailableValue, err := intstr.GetValueFromIntOrPercent(rollingUpdate.MaxUnavailable, int(replicas), true)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Monitoring rollout (maxSurge=%d, maxUnavailable=%d) ===\n", maxSurgeValue, maxUnavailableValue)

		gomega.Eventually(func() error {
			// Get latest deployment status
			deployment, err := clientset.AppsV1().Deployments("test-ns").Get(
				context.TODO(),
				"app",
				metav1.GetOptions{},
			)
			if err != nil {
				return err
			}

			// Check rollout completion
			if deployment.Status.UpdatedReplicas == *deployment.Spec.Replicas &&
				deployment.Status.Replicas == *deployment.Spec.Replicas &&
				deployment.Status.AvailableReplicas == *deployment.Spec.Replicas {
				return nil // Rollout complete
			}

			// Monitor pod states
			pods, err := clientset.CoreV1().Pods("test-ns").List(context.TODO(), metav1.ListOptions{
				LabelSelector: "app=app", // Match deployment selector
			})
			if err != nil {
				return err
			}

			var terminating, pending, runningNotReady, ready int
			for _, pod := range pods.Items {
				if pod.DeletionTimestamp != nil {
					terminating++
					fmt.Printf("[Terminating] %s\n", pod.Name)
					continue
				}

				switch pod.Status.Phase {
				case v1.PodPending:
					pending++
					fmt.Printf("[Pending] %s\n", pod.Name)
				case v1.PodRunning:
					isReady := false
					for _, cond := range pod.Status.Conditions {
						if cond.Type == v1.PodReady && cond.Status == v1.ConditionTrue {
							isReady = true
							break
						}
					}
					if isReady {
						ready++
						fmt.Printf("[Ready] %s\n", pod.Name)
					} else {
						runningNotReady++
						fmt.Printf("[RunningNotReady] %s\n", pod.Name)
					}
				}
			}

			totalPods := terminating + pending + runningNotReady + ready
			surge := totalPods - int(replicas)
			unavailable := terminating + pending + runningNotReady

			// Validate strategy limits
			if surge > maxSurgeValue {
				return fmt.Errorf("surge violation: %d > %d (maxSurge)", surge, maxSurgeValue)
			}
			if unavailable > maxUnavailableValue {
				return fmt.Errorf("unavailable violation: %d > %d (maxUnavailable)", unavailable, maxUnavailableValue)
			}

			fmt.Printf("Pod Status Summary:\n"+
				"  Total: %d\n  Surge: %d/%d\n  Unavailable: %d/%d\n"+
				"  Ready: %d\n  RunningNotReady: %d\n  Pending: %d\n  Terminating: %d\n\n",
				totalPods, surge, maxSurgeValue, unavailable, maxUnavailableValue,
				ready, runningNotReady, pending, terminating)

			return fmt.Errorf("rollout ongoing") // Continue monitoring
		}, 3*time.Minute, 5*time.Second).Should(gomega.Succeed())
	})

})
