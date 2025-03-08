package test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func TestTopologicalSpreadConstraint(t *testing.T) {
	var clientset *kubernetes.Clientset

	// Setup Kubernetes client
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	assert.NoError(t, err)

	clientset, err = kubernetes.NewForConfig(config)
	assert.NoError(t, err)

	// Test case: Create a pod with topological spread constraints
	t.Run("should create a pod with topological spread constraints", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pod",
			},
			Spec: corev1.PodSpec{
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
					{
						MaxSkew:           1,
						TopologyKey:       "kubernetes.io/hostname",
						WhenUnsatisfiable: corev1.DoNotSchedule,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "test"},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:  "test-container",
						Image: "nginx",
					},
				},
			},
		}

		createdPod, err := clientset.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(createdPod.Spec.TopologySpreadConstraints))
	})
}
