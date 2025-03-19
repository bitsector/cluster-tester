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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"example"
)

func TestDeploymentPDB(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Deployment PDB Test Suite")
}

var _ = ginkgo.Describe("Deployment PDB E2E test", ginkgo.Ordered, ginkgo.Label("safe-in-production"), func() {
	var clientset *kubernetes.Clientset
	var minBDPAllowedPods int32
	var logger zerolog.Logger

	ginkgo.BeforeAll(func() {
		fmt.Printf("\n=== Starting Deployment PDB E2E test ===\n")

		var err error
		clientset, err = example.GetClient()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		logger = example.GetLogger("DeploymentPDBTest")
		logger.Info().Msg("Deployment PDB Test zerolog init")

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

	ginkgo.It("should apply PDB manifests", func() {
		pdbYAML, depYAML, err := example.GetPDBDeploymentTestFiles()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		type pdbSpec struct {
			Spec struct {
				MinAvailable int32 `yaml:"minAvailable"`
			} `yaml:"spec"`
		}

		var pdbConfig pdbSpec
		err = yaml.Unmarshal([]byte(pdbYAML), &pdbConfig)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		minBDPAllowedPods = pdbConfig.Spec.MinAvailable
		fmt.Printf("\n=== Minimum allowed pods from PDB: %d ===\n", minBDPAllowedPods)

		// Apply all the manifests
		fmt.Printf("\n=== Applying Deployment manifest ===\n")
		err = example.ApplyRawManifest(clientset, depYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Applying PDB manifest ===\n")
		err = example.ApplyRawManifest(clientset, pdbYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Wait for Pods to schedule ===\n")
		time.Sleep(30 * time.Second)
	})

	ginkgo.It("should maintain minimum pods during rolling update", func() {
		// Get existing deployment
		currentDeployment, err := clientset.AppsV1().Deployments("test-ns").Get(
			context.TODO(),
			"app",
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Create modified deployment with new CPU request
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

		// Monitoring parameters
		const (
			checkInterval = 15 * time.Second
			maxAttempts   = 20
		)
		minObservedPods := int32(1 << 30) // Initialize with very high number
		checkCounter := 1
		rolloutComplete := false

		fmt.Printf("\n=== Starting rolling update monitoring ===\n")
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			// Get current deployment status
			deployment, err := clientset.AppsV1().Deployments("test-ns").Get(
				context.TODO(),
				"app",
				metav1.GetOptions{},
			)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Check rollout completion
			if deployment.Status.UpdatedReplicas == *deployment.Spec.Replicas &&
				deployment.Status.Replicas == *deployment.Spec.Replicas &&
				deployment.Status.AvailableReplicas == *deployment.Spec.Replicas {
				rolloutComplete = true
				fmt.Printf("\n=== Rollout completed successfully ===\n")
				break
			}

			// Get current pods
			checkStart := time.Now()
			runningPods, err := clientset.CoreV1().Pods("test-ns").List(
				context.TODO(),
				metav1.ListOptions{
					FieldSelector: "status.phase=Running",
					LabelSelector: "app=app",
				},
			)
			checkDuration := time.Since(checkStart)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Calculate pod statuses
			var ready, runningNotReady, pending, terminating int
			currentRunningPods := int32(len(runningPods.Items))
			var podNames []string

			for _, pod := range runningPods.Items {
				podNames = append(podNames, pod.Name)
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

			// Update minimum observed runningPods
			if currentRunningPods < minObservedPods {
				minObservedPods = currentRunningPods
			}

			// Get rolling update strategy parameters
			rollingUpdate := deployment.Spec.Strategy.RollingUpdate
			maxSurge := "0"
			maxUnavailable := "0"
			if rollingUpdate != nil {
				maxSurge = rollingUpdate.MaxSurge.String()
				maxUnavailable = rollingUpdate.MaxUnavailable.String()
			}

			// Print detailed status
			fmt.Printf("\n=== Check %d ===\n", checkCounter)
			fmt.Printf("Rollout Status:\n"+
				"  Total Pods: %d\n"+
				"  Surge Usage: %d/%s\n"+
				"  Unavailable: %d/%s\n"+
				"  Ready: %d | RunningNotReady: %d | Pending: %d | Terminating: %d\n"+
				"  Pod Names: %v\n"+
				"  Check Duration: %vms\n",
				len(runningPods.Items),
				len(runningPods.Items)-int(*deployment.Spec.Replicas), maxSurge,
				int(*deployment.Spec.Replicas)-int(deployment.Status.AvailableReplicas), maxUnavailable,
				ready, runningNotReady, pending, terminating,
				podNames,
				checkDuration.Milliseconds())

			// Immediate validation
			gomega.Expect(currentRunningPods).To(
				gomega.BeNumerically(">=", minBDPAllowedPods),
				fmt.Sprintf("Check %d: Running Pod count %d < PDB minimum %d",
					checkCounter,
					currentRunningPods,
					minBDPAllowedPods),
			)

			checkCounter++
			time.Sleep(checkInterval)
		}

		// Final validation
		gomega.Expect(rolloutComplete).To(gomega.BeTrue(), "Rollout did not complete within timeout")
		gomega.Expect(minObservedPods).To(
			gomega.BeNumerically(">=", minBDPAllowedPods),
			fmt.Sprintf("Minimum observed running pods (%d) violated PDB requirement (%d)",
				minObservedPods,
				minBDPAllowedPods),
		)

		fmt.Printf("\n=== Rolling update completed with minimum %d running pods (PDB requires >=%d) ===\n",
			minObservedPods,
			minBDPAllowedPods)
	})

	ginkgo.It("should maintain minimum pod count during deletions", func() {
		// Get current pod count with proper selectors
		labelSelector := "app=app,component=my-unique-deployment"

		pods, err := clientset.CoreV1().Pods("test-ns").List(
			context.TODO(),
			metav1.ListOptions{
				LabelSelector: labelSelector,
				FieldSelector: "status.phase=Running",
			},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Filter out terminating pods in code
		var activePods []v1.Pod
		for _, pod := range pods.Items {
			if pod.DeletionTimestamp == nil {
				activePods = append(activePods, pod)
			}
		}
		initialPods := len(activePods)
		fmt.Printf("\n=== Initial active pods: %d ===\n", initialPods)

		// Verify minimum pod count
		gomega.Expect(int32(initialPods)).To(
			gomega.BeNumerically(">=", minBDPAllowedPods),
			fmt.Sprintf("Initial pods (%d) below PDB minimum (%d)", initialPods, minBDPAllowedPods),
		)

		// Delete all active pods
		fmt.Printf("\n=== Deleting all %d pods ===\n", initialPods)
		for _, pod := range activePods {
			err := clientset.CoreV1().Pods("test-ns").Delete(
				context.TODO(),
				pod.Name,
				metav1.DeleteOptions{},
			)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}

		// Post-deletion checks with proper filtering
		fmt.Printf("\n=== Performing post-deletion validation ===\n")
		const numAttempts = 10
		for attempt := 1; attempt <= numAttempts; attempt++ {
			startPostCheck := time.Now()

			postDeletePods, err := clientset.CoreV1().Pods("test-ns").List(
				context.TODO(),
				metav1.ListOptions{
					LabelSelector: labelSelector,
					FieldSelector: "status.phase=Running",
				},
			)
			postCheckDuration := time.Since(startPostCheck)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Filter terminating pods
			var currentActivePods []v1.Pod
			for _, p := range postDeletePods.Items {
				if p.DeletionTimestamp == nil {
					currentActivePods = append(currentActivePods, p)
				}
			}
			finalCount := len(currentActivePods)

			fmt.Printf("Attempt %d: Active Pods=%d, Sampling Duration=%v\n",
				attempt,
				finalCount,
				postCheckDuration.Round(time.Millisecond))

			gomega.Expect(int32(finalCount)).To(
				gomega.BeNumerically(">=", minBDPAllowedPods),
				fmt.Sprintf("Attempt %d: Pod count %d < PDB minimum %d",
					attempt,
					finalCount,
					minBDPAllowedPods),
			)
		}

		fmt.Printf("\n=== All post-deletion checks passed ===\n")
	})

})
