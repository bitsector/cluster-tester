package example

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// Global variable for kubeconfig path
var KubeconfigPath string

func initKubeconfig() error {
	// Try to load .env file
	err := godotenv.Load(".env")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error loading .env file: %w", err)
	}

	// Get kubeconfig path from environment
	KubeconfigPath = os.Getenv("KUBECONFIG")

	// Fallback to default if not set
	if KubeconfigPath == "" {
		if os.IsNotExist(err) { // .env doesn't exist
			home := homedir.HomeDir()
			if home == "" {
				return fmt.Errorf("no home directory found")
			}
			KubeconfigPath = filepath.Join(home, ".kube", "config")
		} else { // .env exists but KUBECONFIG is empty
			panic(".env file format error, please use KUBECONFIG=/path/to/.kube/config")
		}
	}

	// Verify kubeconfig file exists
	if _, err := os.Stat(KubeconfigPath); err != nil {
		return fmt.Errorf("kubeconfig not found: %w (checked: %s)", err, KubeconfigPath)
	}

	return nil
}

func GetClient() (*kubernetes.Clientset, error) {
	if err := initKubeconfig(); err != nil {
		return nil, err
	}

	config, err := clientcmd.BuildConfigFromFlags("", KubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("config creation error: %w", err)
	}

	return kubernetes.NewForConfig(config)
}

func GetTopologyDeploymentTestFiles() ([]byte, []byte, error) {
	hpaPath := filepath.Join("topology_test_deployment_yamls", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, fmt.Errorf("HPA file error: %w (checked: %s)", err, hpaPath)
	}

	deploymentPath := filepath.Join("topology_test_deployment_yamls", "topology-dep.yaml")
	deploymentContent, err := os.ReadFile(deploymentPath)
	if err != nil {
		return nil, nil, fmt.Errorf("deployment file error: %w (checked: %s)", err, deploymentPath)
	}

	return hpaContent, deploymentContent, nil
}

func GetAffinityDeploymentTestFiles() ([]byte, []byte, []byte, error) {
	hpaPath := filepath.Join("affinity_test_deployment_yamls", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("HPA trigger file error: %w (checked: %s)", err, hpaPath)
	}

	zonePath := filepath.Join("affinity_test_deployment_yamls", "zone-marker.yaml")
	zoneContent, err := os.ReadFile(zonePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("zone marker file error: %w (checked: %s)", err, zonePath)
	}

	deploymentPath := filepath.Join("affinity_test_deployment_yamls", "affinity-dependent-app.yaml")
	deploymentContent, err := os.ReadFile(deploymentPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("affinity-dependent deployment file error: %w (checked: %s)", err, deploymentPath)
	}

	return hpaContent, zoneContent, deploymentContent, nil
}

func GetAntiAffinityTestFiles() ([]byte, []byte, []byte, error) {
	hpaPath := filepath.Join("anti_affinity_test_deployment_yamls", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("HPA trigger file error: %w (checked: %s)", err, hpaPath)
	}

	zonePath := filepath.Join("anti_affinity_test_deployment_yamls", "zone-marker.yaml")
	zoneContent, err := os.ReadFile(zonePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("zone marker file error: %w (checked: %s)", err, zonePath)
	}

	deploymentPath := filepath.Join("anti_affinity_test_deployment_yamls", "anti-affinity-dependent-app.yaml")
	deploymentContent, err := os.ReadFile(deploymentPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("anti-affinity-dependent deployment file error: %w (checked: %s)", err, deploymentPath)
	}

	return hpaContent, zoneContent, deploymentContent, nil
}

func GetPDBTestFiles() ([]byte, []byte, []byte, error) {
	hpaPath := filepath.Join("pdb_deployment_test_yamls", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("HPA trigger file error: %w (checked: %s)", err, hpaPath)
	}

	deploymentPath := filepath.Join("pdb_deployment_test_yamls", "deployment.yaml")
	deploymentContent, err := os.ReadFile(deploymentPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("deployment file error: %w (checked: %s)", err, deploymentPath)
	}

	pdbPath := filepath.Join("pdb_deployment_test_yamls", "pdb.yaml")
	pdbContent, err := os.ReadFile(pdbPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("PDB file error: %w (checked: %s)", err, pdbPath)
	}

	return hpaContent, pdbContent, deploymentContent, nil
}

func GetRollingUpdateTestFiles() ([]byte, []byte, []byte, error) {
	startPath := filepath.Join("rolling_update_test_yamls", "deployment_start.yaml")
	startContent, err := os.ReadFile(startPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("deployment start file error: %w (checked: %s)", err, startPath)
	}

	hpaPath := filepath.Join("rolling_update_test_yamls", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("HPA trigger file error: %w (checked: %s)", err, hpaPath)
	}

	endPath := filepath.Join("rolling_update_test_yamls", "deployment_end.yaml")
	endContent, err := os.ReadFile(endPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("deployment end file error: %w (checked: %s)", err, endPath)
	}

	return startContent, hpaContent, endContent, nil
}

func GetAffinityStatefulSetTestFiles() ([]byte, []byte, []byte, error) {
	hpaPath := filepath.Join("affinity_test_statefulset_yamls", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("HPA trigger file error: %w (checked: %s)", err, hpaPath)
	}

	zonePath := filepath.Join("affinity_test_statefulset_yamls", "zone-marker.yaml")
	zoneContent, err := os.ReadFile(zonePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("zone marker file error: %w (checked: %s)", err, zonePath)
	}

	statefulSetPath := filepath.Join("affinity_test_statefulset_yamls", "affinity-dependent-app.yaml")
	statefulSetContent, err := os.ReadFile(statefulSetPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("affinity-dependent StatefulSet file error: %w (checked: %s)", err, statefulSetPath)
	}

	return hpaContent, zoneContent, statefulSetContent, nil
}

func GetAntiAffinityStatefulSetTestFiles() ([]byte, []byte, []byte, error) {
	hpaPath := filepath.Join("anti_affinity_statefulset_test_yamls", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("HPA trigger file error: %w (checked: %s)", err, hpaPath)
	}

	zonePath := filepath.Join("anti_affinity_statefulset_test_yamls", "zone-marker.yaml")
	zoneContent, err := os.ReadFile(zonePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("zone marker file error: %w (checked: %s)", err, zonePath)
	}

	statefulSetPath := filepath.Join("anti_affinity_statefulset_test_yamls", "anti-affinity-dependent-app.yaml")
	statefulSetContent, err := os.ReadFile(statefulSetPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("anti-affinity-dependent StatefulSet file error: %w (checked: %s)", err, statefulSetPath)
	}

	return hpaContent, zoneContent, statefulSetContent, nil
}

func GetStatefulSetTestFiles() ([]byte, []byte, error) {
	hpaPath := filepath.Join("topology_test_statefulset_yamls", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, fmt.Errorf("HPA file error: %w (checked: %s)", err, hpaPath)
	}

	statefulsetPath := filepath.Join("topology_test_statefulset_yamls", "topology-statefulset.yaml")
	statefulsetContent, err := os.ReadFile(statefulsetPath)
	if err != nil {
		return nil, nil, fmt.Errorf("StatefulSet file error: %w (checked: %s)", err, statefulsetPath)
	}

	return hpaContent, statefulsetContent, nil
}
