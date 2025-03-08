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

func GetTopolgyTestFiles() ([]byte, []byte, error) {
	// Load HPA configuration with appropriate error message
	hpaContent, err := os.ReadFile("./hpa-trigger.yaml")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read HPA file: %w (check file permissions and path)", err)
	}

	// Load Deployment configuration with clear error context
	deploymentContent, err := os.ReadFile("./deployment-with-topology-spread-constraits.yaml")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read deployment file: %w (verify file exists in current directory)", err)
	}

	return hpaContent, deploymentContent, nil
}
