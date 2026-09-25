package aiops

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type trackedActionResource struct {
	GVR       schema.GroupVersionResource
	Namespace string
	Name      string
	Before    *unstructured.Unstructured
	Desired   *unstructured.Unstructured // apply_yaml only
}

func (r trackedActionResource) client(dyn dynamic.Interface) dynamic.ResourceInterface {
	if r.Namespace != "" {
		return dyn.Resource(r.GVR).Namespace(r.Namespace)
	}
	return dyn.Resource(r.GVR)
}

func (r trackedActionResource) fetch(ctx context.Context, dyn dynamic.Interface) (*unstructured.Unstructured, error) {
	obj, err := r.client(dyn).Get(ctx, r.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	return obj, err
}

func actionGVR(action string) (schema.GroupVersionResource, bool) {
	switch action {
	case "create_deployment", "delete_deployment", "scale_deployment", "update_deployment":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, true
	case "create_service", "delete_service":
		return schema.GroupVersionResource{Version: "v1", Resource: "services"}, true
	case "delete_pod":
		return schema.GroupVersionResource{Version: "v1", Resource: "pods"}, true
	case "create_configmap":
		return schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, true
	case "create_secret":
		return schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, true
	case "create_namespace":
		return schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}, true
	case "create_ingress":
		return schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}, true
	case "create_hpa":
		return schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}, true
	case "create_pvc":
		return schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"}, true
	}
	return schema.GroupVersionResource{}, false
}

func snapshotActionResources(ctx context.Context, clusterID uint, params StagedActionParams) (dynamic.Interface, []trackedActionResource, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return nil, nil, fmt.Errorf("cluster not connected: %w", err)
	}
	dyn, err := dynamic.NewForConfig(client.Config)
	if err != nil {
		return nil, nil, err
	}
	var resources []trackedActionResource
	if params.Action == "apply_yaml" {
		manifests, err := parseYAMLResources(client, params)
		if err != nil {
			return nil, nil, err
		}
		for _, manifest := range manifests {
			resources = append(resources, trackedActionResource{GVR: manifest.GVR, Namespace: manifest.Object.GetNamespace(),
				Name: manifest.Object.GetName(), Desired: manifest.Object})
		}
	} else {
		gvr, ok := actionGVR(params.Action)
		if !ok {
			return nil, nil, fmt.Errorf("unsupported action for guarded retry: %s", params.Action)
		}
		ns := params.Namespace
		name := params.Name
		if params.Action == "create_namespace" {
			ns = ""
		}
		if params.Action == "create_hpa" {
			name += "-hpa"
		}
		resources = append(resources, trackedActionResource{GVR: gvr, Namespace: ns, Name: name})
	}
	for i := range resources {
		resources[i].Before, err = resources[i].fetch(ctx, dyn)
		if err != nil {
			return nil, nil, fmt.Errorf("执行前无法核验 %s/%s: %w", resources[i].Namespace, resources[i].Name, err)
		}
		if strings.HasPrefix(params.Action, "create_") && resources[i].Before != nil {
			return nil, nil, fmt.Errorf("%s %s/%s 已存在，需重新预览并确认，不能覆盖", params.Action, resources[i].Namespace, resources[i].Name)
		}
	}
	return dyn, resources, nil
}

func unchangedResource(before, after *unstructured.Unstructured) bool {
	if before == nil || after == nil {
		return before == nil && after == nil
	}
	return before.GetUID() == after.GetUID() && before.GetResourceVersion() == after.GetResourceVersion()
}

func desiredFieldsMatch(desired, actual any) bool {
	switch d := desired.(type) {
	case map[string]any:
		a, ok := actual.(map[string]any)
		if !ok {
			return false
		}
		for key, value := range d {
			if key == "status" || key == "resourceVersion" || key == "uid" || key == "managedFields" || key == "creationTimestamp" {
				continue
			}
			if !desiredFieldsMatch(value, a[key]) {
				return false
			}
		}
		return true
	case []any:
		a, ok := actual.([]any)
		if !ok || len(d) != len(a) {
			return false
		}
		for i := range d {
			if !desiredFieldsMatch(d[i], a[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(desired, actual)
	}
}

func actionOutcomeObserved(params StagedActionParams, before, after *unstructured.Unstructured, desired *unstructured.Unstructured) bool {
	switch params.Action {
	case "delete_deployment", "delete_service", "delete_pod":
		return after == nil || after.GetDeletionTimestamp() != nil
	case "scale_deployment":
		if after == nil {
			return false
		}
		replicas, ok, _ := unstructured.NestedInt64(after.Object, "spec", "replicas")
		return ok && replicas == int64(params.Replicas)
	case "update_deployment":
		if after == nil || before == nil || len(params.EnvVars) > 0 || len(params.ResourceLimits) > 0 {
			return false
		}
		containers, ok, _ := unstructured.NestedSlice(after.Object, "spec", "template", "spec", "containers")
		if !ok || len(containers) == 0 {
			return false
		}
		first, ok := containers[0].(map[string]any)
		return ok && first["image"] == params.NewImage
	case "create_namespace":
		return before == nil && after != nil
	case "apply_yaml":
		return before == nil && after != nil && desired != nil && desiredFieldsMatch(desired.Object, after.Object)
	case "create_deployment":
		if before != nil || after == nil {
			return false
		}
		spec, ok, _ := unstructured.NestedMap(after.Object, "spec")
		if !ok {
			return false
		}
		replicas, _, _ := unstructured.NestedInt64(spec, "replicas")
		wantReplicas := int64(params.Replicas)
		if wantReplicas <= 0 {
			wantReplicas = 1
		}
		containers, _, _ := unstructured.NestedSlice(spec, "template", "spec", "containers")
		if len(containers) == 0 {
			return false
		}
		first, ok := containers[0].(map[string]any)
		if !ok || first["image"] != params.Image || replicas != wantReplicas {
			return false
		}
		ports, _ := first["ports"].([]any)
		if len(ports) != len(params.Ports) {
			return false
		}
		for i, want := range params.Ports {
			port, ok := ports[i].(map[string]any)
			if !ok || port["containerPort"] != int64(want) {
				return false
			}
		}
		volumes, _, _ := unstructured.NestedSlice(spec, "template", "spec", "volumes")
		mounts, _ := first["volumeMounts"].([]any)
		if len(volumes) != len(params.HostPathMounts) || len(mounts) != len(params.HostPathMounts) {
			return false
		}
		for i, mount := range params.HostPathMounts {
			volume, ok := volumes[i].(map[string]any)
			if !ok {
				return false
			}
			hostPath, ok := volume["hostPath"].(map[string]any)
			if !ok || hostPath["path"] != mount.HostPath {
				return false
			}
			volumeMount, ok := mounts[i].(map[string]any)
			if !ok || volumeMount["mountPath"] != mount.MountPath {
				return false
			}
			if mount.ReadOnly && volumeMount["readOnly"] != true {
				return false
			}
		}
		return true
	case "create_service":
		if before != nil || after == nil {
			return false
		}
		selector, _, _ := unstructured.NestedStringMap(after.Object, "spec", "selector")
		portList, _, _ := unstructured.NestedSlice(after.Object, "spec", "ports")
		if !reflect.DeepEqual(selector, params.Selector) || len(portList) == 0 {
			return false
		}
		port, ok := portList[0].(map[string]any)
		if !ok {
			return false
		}
		portNum, _ := port["port"].(int64)
		nodePort, _ := port["nodePort"].(int64)
		serviceType, _, _ := unstructured.NestedString(after.Object, "spec", "type")
		wantType := params.ServiceType
		if params.NodePort > 0 {
			wantType = "NodePort"
		} else if wantType == "" {
			wantType = "ClusterIP"
		}
		targetPort := params.TargetPort
		if targetPort <= 0 {
			targetPort = params.Port
		}
		actualTargetPort, _ := port["targetPort"].(int64)
		return portNum == int64(params.Port) && actualTargetPort == int64(targetPort) &&
			serviceType == wantType && (params.NodePort == 0 || nodePort == int64(params.NodePort))
	case "create_configmap":
		if before != nil || after == nil {
			return false
		}
		data, _, _ := unstructured.NestedStringMap(after.Object, "data")
		return reflect.DeepEqual(data, params.Data)
	case "create_secret":
		if before != nil || after == nil {
			return false
		}
		data, _, _ := unstructured.NestedStringMap(after.Object, "data")
		if len(data) != len(params.Data) {
			return false
		}
		for key, value := range params.Data {
			if data[key] != base64.StdEncoding.EncodeToString([]byte(value)) {
				return false
			}
		}
		return true
	}
	return false // Other creates require a human to resolve any ambiguous result.
}

// ExecuteStagedActionWithRetry retries transient writes only after a live-state
// check proves the prior attempt did not change any tracked resource.
func (s *Service) ExecuteStagedActionWithRetry(ctx context.Context, action *model.AgentAction) (*ExecuteResult, error) {
	var params StagedActionParams
	if err := json.Unmarshal([]byte(action.Parameters), &params); err != nil {
		return nil, fmt.Errorf("invalid staged parameters: %w", err)
	}
	dyn, resources, err := snapshotActionResources(ctx, action.ClusterID, params)
	if err != nil {
		return &ExecuteResult{Success: false, Message: err.Error()}, nil
	}
	for attempt := 0; attempt < 3; attempt++ {
		result, execErr := s.ExecuteStagedAction(ctx, action)
		if execErr == nil && result != nil && result.Success {
			return result, nil
		}
		failure := "execution failed"
		if execErr != nil {
			failure = execErr.Error()
		} else if result != nil {
			failure = result.Message
		}
		if !isTransientAgentFailure(failure) {
			return result, execErr
		}
		allUnchanged, allObserved := true, true
		for _, resource := range resources {
			after, checkErr := resource.fetch(ctx, dyn)
			if checkErr != nil {
				return &ExecuteResult{Success: false, Message: fmt.Sprintf("%s；重试前核验 %s/%s 失败：%v，已停止以避免重复变更", failure, resource.Namespace, resource.Name, checkErr)}, nil
			}
			allUnchanged = allUnchanged && unchangedResource(resource.Before, after)
			allObserved = allObserved && actionOutcomeObserved(params, resource.Before, after, resource.Desired)
		}
		if allObserved {
			return &ExecuteResult{Success: true, Message: "前次请求虽返回错误，但已核验目标状态：操作已生效，无需重复执行"}, nil
		}
		if !allUnchanged {
			return &ExecuteResult{Success: false, Message: failure + "；资源状态已变化但未能确认全部目标已生效，已停止自动重试，请检查实际状态后重新预览"}, nil
		}
		if attempt == 2 {
			return result, execErr
		}
		timer := time.NewTimer(time.Duration(250<<attempt) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, fmt.Errorf("retry exhausted")
}
