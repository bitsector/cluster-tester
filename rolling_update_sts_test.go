package example_test

import (
	"bytes"
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
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	"example"
)

func TestRollingUpdateStatefulSet(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "StatefulSet Rolling Update Test Suite")
}

var _ = ginkgo.Describe("StatefulSet Rolling Update E2E test", ginkgo.Ordered, func() {
	var (
		clientset   *kubernetes.Clientset
		ssStartYAML []byte
	)

	ginkgo.BeforeAll(func() {
		fmt.Printf("\n=== Starting StatefulSet Rolling Update E2E test ===\n")

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
		ssStartYAML, _, err = example.GetRollingUpdateStatefulSetTestFiles()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Parse YAML to find expected replicas
		var expectedReplicas int32
		decoder := serializer.NewCodecFactory(scheme.Scheme).UniversalDeserializer()

		// Split multi-document YAML [1]
		manifests := bytes.Split(ssStartYAML, []byte("---"))
		for _, manifest := range manifests {
			if len(bytes.TrimSpace(manifest)) == 0 {
				continue
			}

			obj, _, err := decoder.Decode(manifest, nil, nil)
			if err != nil {
				continue // Skip invalid documents
			}

			// Extract replicas from StatefulSet [1][4]
			if sts, ok := obj.(*appsv1.StatefulSet); ok {
				expectedReplicas = *sts.Spec.Replicas
				break
			}
		}

		gomega.Expect(expectedReplicas).To(gomega.BeNumerically(">", 0),
			"No StatefulSet found in manifest")

		// Apply all manifests
		fmt.Printf("\n=== Applying Initial StatefulSet and Service manifest ===\n")
		err = example.ApplyRawManifest(clientset, ssStartYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Waiting for Pods to schedule ===\n")
		time.Sleep(100 * time.Second)

		// Verify current StatefulSet status
		currentSTS, err := clientset.AppsV1().StatefulSets("test-ns").Get(
			context.TODO(),
			"app",
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Validation ===\n")
		fmt.Printf("Expected replicas: %d\n", expectedReplicas)
		fmt.Printf("Current ReadyReplicas: %d\n", currentSTS.Status.ReadyReplicas)

		gomega.Expect(currentSTS.Status.ReadyReplicas).To(
			gomega.Equal(expectedReplicas),
			"Ready replicas (%d) should match expected (%d)",
			currentSTS.Status.ReadyReplicas,
			expectedReplicas,
		)
	})

	ginkgo.It("should perform rolling update with updated CPU requests for StatefulSet", func() {
		fmt.Printf("\n=== Preparing StatefulSet rolling update with new CPU requests ===\n")

		// Get existing StatefulSet
		currentSTS, err := clientset.AppsV1().StatefulSets("test-ns").Get(
			context.TODO(),
			"app",
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Update container spec CPU to new value
		newSTS := currentSTS.DeepCopy()
		newSTS.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceCPU] = resource.MustParse("100m")

		fmt.Printf("\n=== Triggering StatefulSet rolling update ===\n")
		_, err = clientset.AppsV1().StatefulSets("test-ns").Update(
			context.TODO(),
			newSTS,
			metav1.UpdateOptions{
				FieldManager: "e2e-test",
			},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		expectedReplicas := *newSTS.Spec.Replicas
		fmt.Printf("\n=== StatefulSet Replicas: %d ===\n", expectedReplicas)

		rolloutCheckNum := 1
		gomega.Eventually(func() error {
			sts, err := clientset.AppsV1().StatefulSets("test-ns").Get(
				context.TODO(),
				"app",
				metav1.GetOptions{},
			)
			if err != nil {
				return err
			}

			// Check rollout completion
			if sts.Status.UpdatedReplicas == expectedReplicas &&
				sts.Status.Replicas == expectedReplicas &&
				sts.Status.AvailableReplicas == expectedReplicas {
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

			totalPods := len(pods.Items)
			fmt.Printf("\nRollout Status:\n"+
				"  Total Pods: %d\n"+
				"  Ready: %d | RunningNotReady: %d | Pending: %d | Terminating: %d\n\n",
				totalPods,
				ready, runningNotReady, pending, terminating)

			// Validate minimum ready pods requirement
			if ready < int(expectedReplicas)-1 {
				return fmt.Errorf("ready pods %d < %d (replicas-1)", ready, expectedReplicas-1)
			}

			rolloutCheckNum++
			return fmt.Errorf("rollout in progress")
		}, 5*time.Minute, 5*time.Second).Should(gomega.Succeed(), "StatefulSet rollout timed out after 5 minutes")

		// Final status report
		fmt.Printf("\n=== Final Rollout Status ===\n")
		pods, err := clientset.CoreV1().Pods("test-ns").List(context.TODO(), metav1.ListOptions{
			LabelSelector: "app=app",
		})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		var readyFinal, runningNotReadyFinal, pendingFinal, terminatingFinal int
		for _, pod := range pods.Items {
			if pod.DeletionTimestamp != nil {
				terminatingFinal++
				continue
			}

			isReady := false
			for _, cond := range pod.Status.Conditions {
				if cond.Type == v1.PodReady && cond.Status == v1.ConditionTrue {
					isReady = true
					break
				}
			}

			switch pod.Status.Phase {
			case v1.PodPending:
				pendingFinal++
			case v1.PodRunning:
				if isReady {
					readyFinal++
				} else {
					runningNotReadyFinal++
				}
			}
		}

		fmt.Printf(
			"  Total Pods: %d\n"+
				"  Ready: %d | RunningNotReady: %d | Pending: %d | Terminating: %d\n\n",
			len(pods.Items),
			readyFinal, runningNotReadyFinal, pendingFinal, terminatingFinal,
		)
	})

})
