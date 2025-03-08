package example

import (
	"context"
	"crypto/rand"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
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
	default:
		return fmt.Errorf("unsupported resource type: %T", obj)
	}

	if err != nil {
		return fmt.Errorf("API server error: %w", err)
	}
	return nil
}

func GetNsName() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 4)

	_, err := rand.Read(b) // Uses crypto/rand for better randomness
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return "test-ns-" + string(b), nil
}
