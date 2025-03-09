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

func TestPDB(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Affinity Test Suite")
}

var _ = ginkgo.Describe("PDB E2E test", ginkgo.Ordered, func() {
	var clientset *kubernetes.Clientset
	var hpaMaxReplicas int32
	var initialNumPods int

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
		hpaYAML, pdbYYAML, depYAML, err := example.GetPDBTestFiles()
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

		fmt.Printf("\n=== Applying PDB manifest ===\n")
		err = example.ApplyRawManifest(clientset, pdbYYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Applying Deployment manifest ===\n")
		err = example.ApplyRawManifest(clientset, depYAML)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Wait for HPA to be triggered ===\n")
		time.Sleep(200 * time.Second)
	})

	ginkgo.It("should attempt to delete all pods in test-ns", func() {
		pods, err := clientset.CoreV1().Pods("test-ns").List(
			context.TODO(),
			metav1.ListOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		initialNumPods = len(pods.Items)

		fmt.Printf("\n=== Attempting to delete %d pods ===\n", len(pods.Items))
		for _, pod := range pods.Items {
			fmt.Printf("Deleting pod: %s\n", pod.Name)
			err := clientset.CoreV1().Pods("test-ns").Delete(
				context.TODO(),
				pod.Name,
				metav1.DeleteOptions{},
			)
			if err != nil {
				fmt.Printf("Failed to delete pod %s: %v\n", pod.Name, err)
			} else {
				fmt.Printf("Successfully initiated deletion of pod %s\n", pod.Name)
			}
		}
	})

	ginkgo.It("should measure pod listing latency", func() {
		fmt.Printf("\n=== Starting pod listing latency test ===\n")
		testStartTime := time.Now()
		for i := 0; ; {
			start := time.Now()
			pods, err := clientset.CoreV1().Pods("test-ns").List(
				context.TODO(),
				metav1.ListOptions{},
			)
			latency := time.Since(start)

			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			fmt.Printf("Attempt %d | Latency: %v | Pod count: %d\n",
				i+1, latency, len(pods.Items))

			if len(pods.Items) > 0 {
				fmt.Println("Current pods:")
				for _, pod := range pods.Items {
					fmt.Printf("  - %s (Phase: %s)\n", pod.Name, pod.Status.Phase)
				}
			}
			if len(pods.Items) >= initialNumPods {
				fmt.Printf("\n=== Pod number recovered to at least original demand %d, continuing... ===\n", len(pods.Items))
				break
			}
			if time.Since(testStartTime) > 10*time.Second {
				fmt.Printf("\n=== Pod number didnt recover to original yet (currently: %d, original: %d), contnuing... ===\n", len(pods.Items), initialNumPods)
				break
			}

			time.Sleep(500 * time.Millisecond) // Brief pause between attempts
		}
	})

	ginkgo.It("should modify existing deployment", func() {
		deploymentName := "app"

		// Get current deployment
		deploy, err := clientset.AppsV1().Deployments("test-ns").Get(
			context.TODO(),
			deploymentName,
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Modify replicas
		fmt.Printf("\n=== Updating replicas from %d to 3 ===\n", *deploy.Spec.Replicas)
		*deploy.Spec.Replicas = 3

		// Modify container image
		containerFound := false
		for i, container := range deploy.Spec.Template.Spec.Containers {
			if container.Name == "main-app" {
				fmt.Printf("Updating image from %s to busybox:latest\n", container.Image)
				deploy.Spec.Template.Spec.Containers[i].Image = "busybox:latest"
				containerFound = true
				break
			}
		}
		gomega.Expect(containerFound).To(gomega.BeTrue(), "Main container not found")

		// Apply updates
		_, err = clientset.AppsV1().Deployments("test-ns").Update(
			context.TODO(),
			deploy,
			metav1.UpdateOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// Verify changes
		updatedDeploy, err := clientset.AppsV1().Deployments("test-ns").Get(
			context.TODO(),
			deploymentName,
			metav1.GetOptions{},
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		fmt.Printf("\n=== Update Verification ===\n")
		fmt.Printf("Current Replicas: %d\n", *updatedDeploy.Spec.Replicas)
		for _, c := range updatedDeploy.Spec.Template.Spec.Containers {
			if c.Name == "main-app" {
				fmt.Printf("Current Image: %s\n", c.Image)
			}
		}
	})
	ginkgo.It("wait", func() {
		time.Sleep(200 * time.Second)

	})

})
