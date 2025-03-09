package example

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	policyv1 "k8s.io/api/policy/v1" // Add this import
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/kubernetes"
)

var (
	scheme         = runtime.NewScheme()
	jsonSerializer = json.NewSerializerWithOptions(
		json.DefaultMetaFactory, scheme, scheme,
		json.SerializerOptions{Yaml: false, Strict: true},
	)
	yamlSerializer = yaml.NewDecodingSerializer(jsonSerializer)
)

func init() {
	appsv1.AddToScheme(scheme)
	autoscalingv2.AddToScheme(scheme)
	policyv1.AddToScheme(scheme) // Add PDB API types
}

func ApplyRawManifest(clientset *kubernetes.Clientset, yamlContent []byte) error {
	obj, _, err := yamlSerializer.Decode(yamlContent, nil, nil)
	if err != nil {
		return fmt.Errorf("YAML decode failed: %w", err)
	}

	switch o := obj.(type) {
	case *autoscalingv2.HorizontalPodAutoscaler:
		_, err = clientset.AutoscalingV2().HorizontalPodAutoscalers(o.Namespace).Create(
			context.TODO(), o, metav1.CreateOptions{})
	case *appsv1.Deployment:
		_, err = clientset.AppsV1().Deployments(o.Namespace).Create(
			context.TODO(), o, metav1.CreateOptions{})
	case *policyv1.PodDisruptionBudget: // Add PDB case
		_, err = clientset.PolicyV1().PodDisruptionBudgets(o.Namespace).Create(
			context.TODO(), o, metav1.CreateOptions{})
	default:
		return fmt.Errorf("unsupported resource type: %T", obj)
	}

	if err != nil {
		return fmt.Errorf("API server error: %w", err)
	}
	return nil
}
