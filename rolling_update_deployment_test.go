package example_test

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/rs/zerolog"
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

var _ = ginkgo.Describe("Deployment Rolling Update E2E test", ginkgo.Ordered, ginkgo.Label("safe-in-production"), func() {
	var (
		clientset    *kubernetes.Clientset
		depStartYAML []byte
		logger       zerolog.Logger
		testTag      = "DeploymentRollingUpdateTest"
	)

	ginkgo.BeforeAll(func() {

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

	ginkgo.It("should apply Rolling update manifests", func() {
		logger.Info().Msgf("=== Starting Deployment Rolling Update E2E test ===")
		logger.Info().Msgf("=== tag: %s, allowed to fail: %t", testTag, example.IsTestAllowedToFail(testTag))
		defer example.E2ePanicHandler()

		var err error
		depStartYAML, err = example.GetRollingUpdateDeploymentTestFiles()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Apply all the manifests
		logger.Info().Msgf("=== Applying Initial deployment manifest ===")
		err = example.ApplyRawManifest(clientset, depStartYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		logger.Info().Msgf("=== Wait for Pods to Schedule ===")
		time.Sleep(30 * time.Second)
	})

	ginkgo.It("should perform rolling update with updated CPU requests", func() {
		defer example.E2ePanicHandler()

		logger.Info().Msgf("=== Preparing rolling update with new CPU requests ===")
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

		logger.Info().Msgf("=== Triggering rolling update with new CPU requests ===")
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

		logger.Info().Msgf("=== Deployment Strategy Configuration ==="+
			"  Replicas: %d\n"+
			"  MaxSurge: %s (%d pods)\n"+
			"  MaxUnavailable: %s (%d pods)\n"+
			"  MinReadySeconds: %d\n\n",
			replicas,
			rollingUpdate.MaxSurge.String(), maxSurgeValue,
			rollingUpdate.MaxUnavailable.String(), maxUnavailableValue,
			minReadySeconds)

		rolloutCheckNum := 1
		lastLog := time.Now()
		logInterval := 5 * time.Second

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
				logger.Info().Msgf("=== Rollout complete ===")
				return nil // Rollout complete
			}

			// Monitor pod states
			pods, err := clientset.CoreV1().Pods("test-ns").List(context.TODO(), metav1.ListOptions{
				LabelSelector: "app=app",
			})
			if err != nil {
				return err
			}
			if time.Since(lastLog) > logInterval {
				logger.Info().Msgf("=== Sample checking rolling update status (attempt %d): ===\n", rolloutCheckNum)
			}
			var terminating, pending, runningNotReady, ready int
			for _, pod := range pods.Items {
				if pod.DeletionTimestamp != nil {
					terminating++
					if time.Since(lastLog) > logInterval {
						logger.Info().Msgf("[Terminating] %s\n", pod.Name)
					}
					continue
				}

				switch pod.Status.Phase {
				case v1.PodPending:
					pending++
					if time.Since(lastLog) > logInterval {
						logger.Info().Msgf("[Pending] %s\n", pod.Name)
					}
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
						if time.Since(lastLog) > logInterval {
							logger.Info().Msgf("[Ready] %s\n", pod.Name)
						}
					} else {
						runningNotReady++
						if time.Since(lastLog) > logInterval {
							logger.Info().Msgf("[RunningNotReady] %s\n", pod.Name)
						}
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
			if time.Since(lastLog) > logInterval {

				logger.Info().Msgf("\nRollout Status:\n"+
					"  Total Pods: %d\n"+
					"  Surge Usage: %d/%s\n"+
					"  Unavailable: %d/%s\n"+
					"  Ready: %d | RunningNotReady: %d | Pending: %d | Terminating: %d\n\n",
					totalPods,
					surge, rollingUpdate.MaxSurge.String(),
					unavailable, rollingUpdate.MaxUnavailable.String(),
					ready, runningNotReady, pending, terminating)
				lastLog = time.Now()
			}
			return fmt.Errorf("rollout in progress")
		}, 5*time.Minute, 10*time.Millisecond).Should(gomega.Succeed())

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

		logger.Info().Msgf("=== Final Rollout Status ==="+
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
