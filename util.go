package example

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	// Register all required API groups
	corev1.AddToScheme(scheme)
	appsv1.AddToScheme(scheme)
	autoscalingv2.AddToScheme(scheme)
	policyv1.AddToScheme(scheme)
}

func ApplyRawManifest(clientset *kubernetes.Clientset, yamlContent []byte) error {
	// Split YAML into individual documents
	documents := bytes.Split(yamlContent, []byte("\n---\n"))
	var errors []string

	for i, doc := range documents {
		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}

		obj, _, err := yamlSerializer.Decode(doc, nil, nil)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Document %d decode failed: %v", i+1, err))
			continue
		}

		var createErr error
		switch o := obj.(type) {
		case *autoscalingv2.HorizontalPodAutoscaler:
			_, createErr = clientset.AutoscalingV2().HorizontalPodAutoscalers(o.Namespace).Create(
				context.TODO(), o, metav1.CreateOptions{})
		case *appsv1.Deployment:
			_, createErr = clientset.AppsV1().Deployments(o.Namespace).Create(
				context.TODO(), o, metav1.CreateOptions{})
		case *appsv1.StatefulSet:
			_, createErr = clientset.AppsV1().StatefulSets(o.Namespace).Create(
				context.TODO(), o, metav1.CreateOptions{})
		case *corev1.Service:
			_, createErr = clientset.CoreV1().Services(o.Namespace).Create(
				context.TODO(), o, metav1.CreateOptions{})
		case *policyv1.PodDisruptionBudget:
			_, createErr = clientset.PolicyV1().PodDisruptionBudgets(o.Namespace).Create(
				context.TODO(), o, metav1.CreateOptions{})
		default:
			errors = append(errors, fmt.Sprintf("Document %d: unsupported type %T", i+1, obj))
			continue
		}

		if createErr != nil {
			errors = append(errors, fmt.Sprintf("Document %d apply failed: %v", i+1, createErr))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("manifest application errors:\n%s", strings.Join(errors, "\n"))
	}
	return nil
}

func ClearNamespace(logger zerolog.Logger, clientset *kubernetes.Clientset) {
	logger.Info().Msgf("=== Final namespace cleanup ===")
	err := clientset.CoreV1().Namespaces().Delete(
		context.TODO(),
		"test-ns",
		metav1.DeleteOptions{},
	)
	if err != nil && !apierrors.IsNotFound(err) {
		logger.Error().Msgf("Initial cleanup failed: %v", err)
	}

	// Wait for initial deletion (3 minutes)
	initialDeleteTimeout := time.Now().Add(3 * time.Minute)
	for {
		_, err := clientset.CoreV1().Namespaces().Get(context.TODO(), "test-ns", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			logger.Info().Msgf("Namespace 'test-ns' successfully deleted")
			return
		}
		if time.Now().After(initialDeleteTimeout) {
			logger.Info().Msgf("Initial deletion timed out after 3 minutes. Attempting force deletion...")
			break
		}
		logger.Info().Msgf("Waiting for initial deletion to complete...")
		time.Sleep(5 * time.Second)
	}

	// Force deletion
	deletePolicy := metav1.DeletePropagationBackground
	deleteOptions := metav1.DeleteOptions{
		GracePeriodSeconds: new(int64),
		PropagationPolicy:  &deletePolicy,
	}
	*deleteOptions.GracePeriodSeconds = 0 // This the forcing part

	err = clientset.CoreV1().Namespaces().Delete(
		context.TODO(),
		"test-ns",
		deleteOptions,
	)
	if err != nil {
		logger.Error().Msgf("Force deletion failed: %v", err)
	}

	// Wait for force deletion (3 minutes)
	forceDeleteTimeout := time.Now().Add(3 * time.Minute)
	for {
		_, err := clientset.CoreV1().Namespaces().Get(context.TODO(), "test-ns", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			logger.Info().Msgf("Namespace 'test-ns' successfully force deleted")
			return
		}
		if time.Now().After(forceDeleteTimeout) {
			logger.Error().Msgf("Force deletion timed out after 3 minutes")
			return
		}
		logger.Info().Msgf("Waiting for force deletion to complete...")
		time.Sleep(5 * time.Second)
	}
}
