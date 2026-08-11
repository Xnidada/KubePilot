package backup

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kubepilot/kubepilot/internal/k8s"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const veleroNamespace = "velero"

var (
	veleroBackupGVR = schema.GroupVersionResource{
		Group: "velero.io", Version: "v1", Resource: "backups",
	}
	veleroRestoreGVR = schema.GroupVersionResource{
		Group: "velero.io", Version: "v1", Resource: "restores",
	}
)

func dynamicClientFor(clusterID uint) (dynamic.Interface, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(client.Config)
}

// VeleroAvailable reports whether the Velero Backup CRD exists in the cluster.
func VeleroAvailable(ctx context.Context, clusterID uint) (bool, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return false, err
	}
	resources, err := client.Discovery.ServerResourcesForGroupVersion("velero.io/v1")
	if err != nil {
		if apierrors.IsNotFound(err) || strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}
	for _, r := range resources.APIResources {
		if r.Name == "backups" {
			return true, nil
		}
	}
	return false, nil
}

func createVeleroBackup(ctx context.Context, clusterID uint, name, ttl, storageLocation string, namespaces, resources []string) error {
	dyn, err := dynamicClientFor(clusterID)
	if err != nil {
		return err
	}
	spec := map[string]interface{}{
		"ttl": ttl,
	}
	if loc := strings.TrimSpace(storageLocation); loc != "" {
		spec["storageLocation"] = loc
	}
	if len(namespaces) > 0 {
		spec["includedNamespaces"] = namespaces
	}
	if len(resources) > 0 {
		spec["includedResources"] = resources
	}
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Backup",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": veleroNamespace,
			},
			"spec": spec,
		},
	}
	_, err = dyn.Resource(veleroBackupGVR).Namespace(veleroNamespace).Create(ctx, obj, metav1.CreateOptions{})
	return err
}

func waitVeleroBackup(ctx context.Context, clusterID uint, name string, timeout time.Duration) (phase string, err error) {
	dyn, err := dynamicClientFor(clusterID)
	if err != nil {
		return "", err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		obj, getErr := dyn.Resource(veleroBackupGVR).Namespace(veleroNamespace).Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return "", getErr
		}
		phase, _, _ = unstructured.NestedString(obj.Object, "status", "phase")
		switch phase {
		case "Completed", "PartiallyFailed", "Failed", "FailedValidation":
			return phase, nil
		}
		time.Sleep(3 * time.Second)
	}
	return phase, fmt.Errorf("timed out waiting for velero backup %s", name)
}

// deleteVeleroBackup removes the Velero Backup CR if present. NotFound is ignored.
func deleteVeleroBackup(ctx context.Context, clusterID uint, name string) error {
	dyn, err := dynamicClientFor(clusterID)
	if err != nil {
		return err
	}
	err = dyn.Resource(veleroBackupGVR).Namespace(veleroNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// deleteVeleroRestore removes the Velero Restore CR if present. NotFound is ignored.
func deleteVeleroRestore(ctx context.Context, clusterID uint, name string) error {
	dyn, err := dynamicClientFor(clusterID)
	if err != nil {
		return err
	}
	err = dyn.Resource(veleroRestoreGVR).Namespace(veleroNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func createVeleroRestore(ctx context.Context, clusterID uint, restoreName, backupName string, namespaces []string) error {
	dyn, err := dynamicClientFor(clusterID)
	if err != nil {
		return err
	}
	spec := map[string]interface{}{
		"backupName": backupName,
	}
	if len(namespaces) > 0 {
		spec["includedNamespaces"] = namespaces
	}
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Restore",
			"metadata": map[string]interface{}{
				"name":      restoreName,
				"namespace": veleroNamespace,
			},
			"spec": spec,
		},
	}
	_, err = dyn.Resource(veleroRestoreGVR).Namespace(veleroNamespace).Create(ctx, obj, metav1.CreateOptions{})
	return err
}

func waitVeleroRestore(ctx context.Context, clusterID uint, name string, timeout time.Duration) (phase string, err error) {
	dyn, err := dynamicClientFor(clusterID)
	if err != nil {
		return "", err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		obj, getErr := dyn.Resource(veleroRestoreGVR).Namespace(veleroNamespace).Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return "", getErr
		}
		phase, _, _ = unstructured.NestedString(obj.Object, "status", "phase")
		switch phase {
		case "Completed", "PartiallyFailed", "Failed", "FailedValidation":
			return phase, nil
		}
		time.Sleep(3 * time.Second)
	}
	return phase, fmt.Errorf("timed out waiting for velero restore %s", name)
}
