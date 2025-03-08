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
