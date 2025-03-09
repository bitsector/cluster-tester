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
	hpaPath := filepath.Join("topology_test_yamls", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, fmt.Errorf("HPA file error: %w (checked: %s)", err, hpaPath)
	}

	deploymentPath := filepath.Join("topology_test_yamls", "topology-dep.yaml")
	deploymentContent, err := os.ReadFile(deploymentPath)
	if err != nil {
		return nil, nil, fmt.Errorf("deployment file error: %w (checked: %s)", err, deploymentPath)
	}

	return hpaContent, deploymentContent, nil
}

func GetAffinityTestFiles() ([]byte, []byte, []byte, error) {
	hpaPath := filepath.Join("affinity_test_yamls", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("HPA trigger file error: %w (checked: %s)", err, hpaPath)
	}

	zonePath := filepath.Join("affinity_test_yamls", "zone-marker.yaml")
	zoneContent, err := os.ReadFile(zonePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("zone marker file error: %w (checked: %s)", err, zonePath)
	}

	deploymentPath := filepath.Join("affinity_test_yamls", "affinity-dependent-app.yaml")
	deploymentContent, err := os.ReadFile(deploymentPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("affinity-dependent deployment file error: %w (checked: %s)", err, deploymentPath)
	}

	return hpaContent, zoneContent, deploymentContent, nil
}

func GetAntiAffinityTestFiles() ([]byte, []byte, []byte, error) {
	hpaPath := filepath.Join("anti_affinity_test_yamls", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("HPA trigger file error: %w (checked: %s)", err, hpaPath)
	}

	zonePath := filepath.Join("anti_affinity_test_yamls", "zone-marker.yaml")
	zoneContent, err := os.ReadFile(zonePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("zone marker file error: %w (checked: %s)", err, zonePath)
	}

	deploymentPath := filepath.Join("anti_affinity_test_yamls", "anti-affinity-dependent-app.yaml")
	deploymentContent, err := os.ReadFile(deploymentPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("anti-affinity-dependent deployment file error: %w (checked: %s)", err, deploymentPath)
	}

	return hpaContent, zoneContent, deploymentContent, nil
}
