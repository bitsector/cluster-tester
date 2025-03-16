package example_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"example"
)

func TestRollingUpdateDeployment(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Deployment Rolling Update Test Suite")
}

var _ = ginkgo.Describe("Deployment Rolling Update E2E test", ginkgo.Ordered, func() {
	var (
		clientset    *kubernetes.Clientset
		depStartYAML []byte
	)

	ginkgo.BeforeAll(func() {
		fmt.Printf("\n=== Starting Deployment Rolling Update E2E test ===\n")

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

	ginkgo.It("should apply Rolling update manifests", func() {
		var err error
		depStartYAML, err = example.GetRollingUpdateDeploymentTestFiles()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Apply all the manifests
		fmt.Printf("\n=== Applying Initial deployment manifest ===\n")
		err = example.ApplyRawManifest(clientset, depStartYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Wait for Pods to Schedule ===\n")
		time.Sleep(30 * time.Second)
	})

	ginkgo.It("should perform rolling update with updated CPU requests", func() {
		fmt.Printf("\n=== Preparing rolling update with new CPU requests ===\n")

		// Get existing deployment
		currentDeployment, err := clientset.AppsV1().Deployments("test-ns").Get(
			context.TODO(),
			"app",
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Update container spec CPU to new value
		newDeployment := currentDeployment.DeepCopy()
		newDeployment.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceCPU] = resource.MustParse("100m")

		fmt.Printf("\n=== Triggering rolling update with new CPU requests ===\n")
		_, err = clientset.AppsV1().Deployments("test-ns").Update(
			context.TODO(),
			newDeployment,
			metav1.UpdateOptions{
				FieldManager: "e2e-test",
			},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Retrieve deployment with updated configuration
		deployment, err := clientset.AppsV1().Deployments("test-ns").Get(
			context.TODO(),
			"app",
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Validate deployment strategy configuration
		if deployment.Spec.Strategy.Type != appsv1.RollingUpdateDeploymentStrategyType {
			ginkgo.Fail("Deployment is not using RollingUpdate strategy")
		}

		rollingUpdate := deployment.Spec.Strategy.RollingUpdate
		if rollingUpdate == nil {
			ginkgo.Fail("Deployment missing RollingUpdate configuration")
		}

		// Get strategy parameters
		replicas := *deployment.Spec.Replicas
		minReadySeconds := deployment.Spec.MinReadySeconds

		maxSurgeValue, err := intstr.GetValueFromIntOrPercent(rollingUpdate.MaxSurge, int(replicas), true)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		maxUnavailableValue, err := intstr.GetValueFromIntOrPercent(rollingUpdate.MaxUnavailable, int(replicas), true)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Deployment Strategy Configuration ===\n"+
			"  Replicas: %d\n"+
			"  MaxSurge: %s (%d pods)\n"+
			"  MaxUnavailable: %s (%d pods)\n"+
			"  MinReadySeconds: %d\n\n",
			replicas,
			rollingUpdate.MaxSurge.String(), maxSurgeValue,
			rollingUpdate.MaxUnavailable.String(), maxUnavailableValue,
			minReadySeconds)

		rolloutCheckNum := 1
		gomega.Eventually(func() error {
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
				LabelSelector: "app=app",
			})
			if err != nil {
				return err
			}

			fmt.Printf("\n=== Sample checking rolling update status (attempt %d): ===\n\n", rolloutCheckNum)
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
				return fmt.Errorf("maxSurge violation: %d > %d (from %s)",
					surge, maxSurgeValue, rollingUpdate.MaxSurge)
			}
			if unavailable > maxUnavailableValue {
				return fmt.Errorf("maxUnavailable violation: %d > %d (from %s)",
					unavailable, maxUnavailableValue, rollingUpdate.MaxUnavailable)
			}

			rolloutCheckNum++

			fmt.Printf("\nRollout Status:\n"+
				"  Total Pods: %d\n"+
				"  Surge Usage: %d/%s\n"+
				"  Unavailable: %d/%s\n"+
				"  Ready: %d | RunningNotReady: %d | Pending: %d | Terminating: %d\n\n",
				totalPods,
				surge, rollingUpdate.MaxSurge.String(),
				unavailable, rollingUpdate.MaxUnavailable.String(),
				ready, runningNotReady, pending, terminating)

			return fmt.Errorf("rollout in progress")
		}, 5*time.Minute, 5*time.Second).Should(gomega.Succeed())

		// Final status check after successful rollout
		ginkgo.By("Final rollout status verification")
		deployment, err = clientset.AppsV1().Deployments("test-ns").Get(
			context.TODO(),
			"app",
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		pods, err := clientset.CoreV1().Pods("test-ns").List(context.TODO(), metav1.ListOptions{
			LabelSelector: "app=app",
		})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		var terminating, pending, runningNotReady, ready int
		for _, pod := range pods.Items {
			if pod.DeletionTimestamp != nil {
				terminating++
				continue
			}

			switch pod.Status.Phase {
			case v1.PodPending:
				pending++
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
				} else {
					runningNotReady++
				}
			}
		}

		totalPods := terminating + pending + runningNotReady + ready
		surge := totalPods - int(*deployment.Spec.Replicas)
		unavailable := terminating + pending + runningNotReady

		fmt.Printf("\n=== Final Rollout Status ===\n"+
			"  Total Pods: %d\n"+
			"  Surge Usage: %d/%s\n"+
			"  Unavailable: %d/%s\n"+
			"  Ready: %d | RunningNotReady: %d | Pending: %d | Terminating: %d\n\n",
			totalPods,
			surge, deployment.Spec.Strategy.RollingUpdate.MaxSurge.String(),
			unavailable, deployment.Spec.Strategy.RollingUpdate.MaxUnavailable.String(),
			ready, runningNotReady, pending, terminating)
	})

})
