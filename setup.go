package example

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func GetClient() (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error

	// First try in-cluster config
	config, err = rest.InClusterConfig()
	if err != nil {
		// Fallback to kubeconfig
		home := homedir.HomeDir()
		if home == "" {
			return nil, fmt.Errorf("no home directory found")
		}

		kubeconfig := filepath.Join(home, ".kube", "config")
		if _, err = os.Stat(kubeconfig); err != nil {
			return nil, fmt.Errorf("kubeconfig not found: %w", err)
		}

		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	if err != nil {
		return nil, fmt.Errorf("config creation error: %w", err)
	}

	return kubernetes.NewForConfig(config)
}

func GetTopologyTestFiles() ([]byte, []byte, error) {
	// Fixed typo in "constraints" and added absolute paths
	hpaPath := filepath.Join(".", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, fmt.Errorf("HPA file error: %w (checked: %s)", err, hpaPath)
	}

	deploymentPath := filepath.Join(".", "deployment-with-topology-spread-constraints.yaml")
	deploymentContent, err := os.ReadFile(deploymentPath)
	if err != nil {
		return nil, nil, fmt.Errorf("Deployment file error: %w (checked: %s)", err, deploymentPath)
	}

	return hpaContent, deploymentContent, nil
}
