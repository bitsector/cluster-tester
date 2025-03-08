package example

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
)

// Add this function to handle raw manifest application
func ApplyRawManifest(clientset *kubernetes.Clientset, yamlContent []byte) error {
	// Implementation would use client-go's dynamic client
	// or yaml decoder to apply manifests
	// This is a simplified placeholder implementation
	_, err := clientset.RESTClient().
		Post().
		AbsPath("/apis/cluster.example.com/v1").
		Body(yamlContent).
		DoRaw(context.TODO())

	if err != nil {
		return fmt.Errorf("manifest application failed: %w", err)
	}
	return nil
}
