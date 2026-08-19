package aiops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// HostPathMount maps a node hostPath into the container.
type HostPathMount struct {
	Name      string `json:"name,omitempty"`
	HostPath  string `json:"host_path"`
	MountPath string `json:"mount_path"`
	ReadOnly  bool   `json:"read_only,omitempty"`
	// Type: Directory | DirectoryOrCreate | File | FileOrCreate; default DirectoryOrCreate.
	Type string `json:"type,omitempty"`
}

// StagedActionParams stores the executable payload for a pending AI action.
type StagedActionParams struct {
	Action         string            `json:"action"`
	Namespace      string            `json:"namespace"`
	Name           string            `json:"name"`
	Image          string            `json:"image,omitempty"`
	Replicas       int32             `json:"replicas,omitempty"`
	Ports          []int32           `json:"ports,omitempty"`
	ServiceType    string            `json:"service_type,omitempty"`
	Port           int32             `json:"port,omitempty"`
	TargetPort     int32             `json:"target_port,omitempty"`
	NodePort       int32             `json:"node_port,omitempty"`
	Selector       map[string]string `json:"selector,omitempty"`
	HostPathMounts []HostPathMount   `json:"host_path_mounts,omitempty"`
	// Extended action fields
	Data           map[string]string `json:"data,omitempty"`            // ConfigMap/Secret data
	SecretType     string            `json:"secret_type,omitempty"`     // Secret type (Opaque/TLS/...)
	NewImage       string            `json:"new_image,omitempty"`       // update_deployment: new image
	EnvVars        map[string]string `json:"env_vars,omitempty"`        // update_deployment: env overrides
	ResourceLimits map[string]string `json:"resource_limits,omitempty"` // update_deployment: resource limits
	Host           string            `json:"host,omitempty"`            // Ingress host
	Path           string            `json:"path,omitempty"`            // Ingress path
	BackendService string            `json:"backend_service,omitempty"` // Ingress backend service name
	BackendPort    int32             `json:"backend_port,omitempty"`    // Ingress backend port
	MinReplicas    int32             `json:"min_replicas,omitempty"`    // HPA minReplicas
	MaxReplicas    int32             `json:"max_replicas,omitempty"`    // HPA maxReplicas
	TargetCPU      int32             `json:"target_cpu,omitempty"`      // HPA target CPU utilization percentage
	StorageClass   string            `json:"storage_class,omitempty"`   // PVC storageClassName
	AccessModes    []string          `json:"access_modes,omitempty"`    // PVC accessModes
	StorageSize    string            `json:"storage_size,omitempty"`    // PVC size (e.g. "10Gi")
	YAML           string            `json:"yaml,omitempty"`            // apply_yaml raw YAML
}

// DryRunStagedAction returns a field-level impact preview.
// Create paths use server-side DryRunAll when possible.
func (s *Service) DryRunStagedAction(ctx context.Context, clusterID uint, params StagedActionParams) (string, error) {
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		return "", fmt.Errorf("cluster not connected: %w", err)
	}

	switch params.Action {
	case "create_deployment":
		if strings.TrimSpace(params.Image) == "" {
			return "", fmt.Errorf("image is required for create_deployment")
		}
		if _, err := client.Clientset.AppsV1().Deployments(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{}); err == nil {
			return "", fmt.Errorf("deployment %s/%s already exists", params.Namespace, params.Name)
		}
		if params.Replicas <= 0 {
			params.Replicas = 1
		}
		dep := buildDeploymentObject(params)
		_, err := client.Clientset.AppsV1().Deployments(params.Namespace).Create(ctx, dep, metav1.CreateOptions{
			DryRun: []string{metav1.DryRunAll},
		})
		if err != nil {
			return "", fmt.Errorf("server dry-run failed: %w", err)
		}
		mountLines := "  + volumes: (none)"
		if len(params.HostPathMounts) > 0 {
			var b strings.Builder
			for i, m := range params.HostPathMounts {
				hpType := m.Type
				if hpType == "" {
					hpType = string(corev1.HostPathDirectoryOrCreate)
				}
				b.WriteString(fmt.Sprintf("  + volumes[%d].hostPath.path: %s (type=%s)\n", i, m.HostPath, hpType))
				b.WriteString(fmt.Sprintf("  + containers[0].volumeMounts[%d].mountPath: %s (readOnly=%v)\n", i, m.MountPath, m.ReadOnly))
			}
			b.WriteString("  note: hostPath is node-local; ensure paths exist (or DirectoryOrCreate) on the scheduled node")
			mountLines = strings.TrimRight(b.String(), "\n")
		}
		return fmt.Sprintf(`[dry-run] CREATE Deployment %s/%s
diff:
  + apiVersion: apps/v1
  + kind: Deployment
  + metadata.name: %s
  + metadata.namespace: %s
  + spec.replicas: %d
  + spec.template.spec.containers[0].image: %s
  + spec.template.spec.containers[0].ports: %v
%s
server_dry_run: ok`, params.Namespace, params.Name, params.Name, params.Namespace, params.Replicas, params.Image, params.Ports, mountLines), nil

	case "create_service":
		if len(params.Selector) == 0 {
			return "", fmt.Errorf("selector is required for create_service")
		}
		if params.Port <= 0 {
			return "", fmt.Errorf("port is required for create_service")
		}
		if _, err := client.Clientset.CoreV1().Services(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{}); err == nil {
			return "", fmt.Errorf("service %s/%s already exists", params.Namespace, params.Name)
		}
		svc := buildServiceObject(params)
		_, err := client.Clientset.CoreV1().Services(params.Namespace).Create(ctx, svc, metav1.CreateOptions{
			DryRun: []string{metav1.DryRunAll},
		})
		if err != nil {
			return "", fmt.Errorf("server dry-run failed: %w", err)
		}
		tp := params.TargetPort
		if tp <= 0 {
			tp = params.Port
		}
		nodePortLine := "  + spec.ports[0].nodePort: (auto — NOT SET; external port unknown)"
		if params.NodePort > 0 {
			nodePortLine = fmt.Sprintf("  + spec.ports[0].nodePort: %d", params.NodePort)
		}
		return fmt.Sprintf(`[dry-run] CREATE Service %s/%s
diff:
  + kind: Service
  + metadata.name: %s
  + metadata.namespace: %s
  + spec.type: %s
  + spec.selector: %v
  + spec.ports[0].port: %d
  + spec.ports[0].targetPort: %d
%s
server_dry_run: ok`, params.Namespace, params.Name, params.Name, params.Namespace, params.ServiceType, params.Selector, params.Port, tp, nodePortLine), nil

	case "delete_deployment":
		deploy, err := client.Clientset.AppsV1().Deployments(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("deployment %s/%s not found", params.Namespace, params.Name)
		}
		replicas := int32(0)
		if deploy.Spec.Replicas != nil {
			replicas = *deploy.Spec.Replicas
		}
		img := ""
		if len(deploy.Spec.Template.Spec.Containers) > 0 {
			img = deploy.Spec.Template.Spec.Containers[0].Image
		}
		age := time.Since(deploy.CreationTimestamp.Time).Truncate(time.Second)
		return fmt.Sprintf(`[dry-run] DELETE Deployment %s/%s
impact:
  - replicas: %d (ready=%d available=%d)
  - image: %s
  - labels: %v
  - age: %s
  - will cascade delete owned ReplicaSets/Pods`,
			params.Namespace, params.Name, replicas, deploy.Status.ReadyReplicas, deploy.Status.AvailableReplicas, img, deploy.Labels, age), nil

	case "delete_service":
		svc, err := client.Clientset.CoreV1().Services(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("service %s/%s not found", params.Namespace, params.Name)
		}
		return fmt.Sprintf(`[dry-run] DELETE Service %s/%s
impact:
  - type: %s
  - clusterIP: %s
  - selector: %v
  - ports: %v`, params.Namespace, params.Name, svc.Spec.Type, svc.Spec.ClusterIP, svc.Spec.Selector, svc.Spec.Ports), nil

		case "delete_pod":
			pod, err := client.Clientset.CoreV1().Pods(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
			if err != nil {
				return "", fmt.Errorf("pod %s/%s not found", params.Namespace, params.Name)
			}
			owners := make([]string, 0, len(pod.OwnerReferences))
			deployOwner := ""
			rsOwner := ""
			for _, o := range pod.OwnerReferences {
				owners = append(owners, fmt.Sprintf("%s/%s", o.Kind, o.Name))
				if o.Kind == "ReplicaSet" {
					rsOwner = o.Name
				}
				if o.Kind == "Deployment" {
					deployOwner = o.Name
				}
			}
			if rsOwner != "" && deployOwner == "" {
				if rs, rsErr := client.Clientset.AppsV1().ReplicaSets(params.Namespace).Get(ctx, rsOwner, metav1.GetOptions{}); rsErr == nil {
					for _, o := range rs.OwnerReferences {
						if o.Kind == "Deployment" {
							deployOwner = o.Name
							owners = append(owners, fmt.Sprintf("Deployment/%s", o.Name))
							break
						}
					}
				}
			}
			note := "note: standalone Pod — delete will persist"
			if deployOwner != "" {
				note = fmt.Sprintf("WARNING: 该 Pod 由 Deployment/%s 管理。删除单个 Pod 后控制器会立刻重建新实例，工作负载看起来“没删掉”。若要彻底移除，请改用 delete_deployment（name=%s）。", deployOwner, deployOwner)
			} else if rsOwner != "" {
				note = fmt.Sprintf("WARNING: 该 Pod 由 ReplicaSet/%s 管理，删除后可能被立即重建。", rsOwner)
			}
			return fmt.Sprintf(`[dry-run] DELETE Pod %s/%s
impact:
  - phase: %s
  - node: %s
  - owners: %v
  - labels: %v
%s`,
				params.Namespace, params.Name, pod.Status.Phase, pod.Spec.NodeName, owners, pod.Labels, note), nil

	case "scale_deployment":
		deploy, err := client.Clientset.AppsV1().Deployments(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("deployment %s/%s not found", params.Namespace, params.Name)
		}
		current := int32(0)
		if deploy.Spec.Replicas != nil {
			current = *deploy.Spec.Replicas
		}
		delta := params.Replicas - current
		sign := "+"
		if delta < 0 {
			sign = ""
		}
		return fmt.Sprintf(`[dry-run] SCALE Deployment %s/%s
diff:
  - spec.replicas: %d
  + spec.replicas: %d  (%s%d)
status_snapshot:
  - readyReplicas: %d
  - availableReplicas: %d
  - updatedReplicas: %d
  - unavailableReplicas: %d`,
			params.Namespace, params.Name, current, params.Replicas, sign, delta,
			deploy.Status.ReadyReplicas, deploy.Status.AvailableReplicas, deploy.Status.UpdatedReplicas, deploy.Status.UnavailableReplicas), nil

	case "create_configmap":
		if _, err := client.Clientset.CoreV1().ConfigMaps(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{}); err == nil {
			return "", fmt.Errorf("configmap %s/%s already exists", params.Namespace, params.Name)
		}
		keys := make([]string, 0, len(params.Data))
		for k := range params.Data {
			keys = append(keys, k)
		}
		return fmt.Sprintf("[dry-run] CREATE ConfigMap %s/%s\n  data keys: %v\nserver_dry_run: ok", params.Namespace, params.Name, keys), nil

	case "create_secret":
		if _, err := client.Clientset.CoreV1().Secrets(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{}); err == nil {
			return "", fmt.Errorf("secret %s/%s already exists", params.Namespace, params.Name)
		}
		st := params.SecretType
		if st == "" {
			st = "Opaque"
		}
		keys := make([]string, 0, len(params.Data))
		for k := range params.Data {
			keys = append(keys, k)
		}
		return fmt.Sprintf("[dry-run] CREATE Secret %s/%s\n  type: %s\n  data keys: %v (values will be base64-encoded)\nserver_dry_run: ok", params.Namespace, params.Name, st, keys), nil

	case "update_deployment":
		deploy, err := client.Clientset.AppsV1().Deployments(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("deployment %s/%s not found", params.Namespace, params.Name)
		}
		var diffLines []string
		if params.NewImage != "" && len(deploy.Spec.Template.Spec.Containers) > 0 {
			diffLines = append(diffLines, fmt.Sprintf("  - containers[0].image: %s", deploy.Spec.Template.Spec.Containers[0].Image))
			diffLines = append(diffLines, fmt.Sprintf("  + containers[0].image: %s", params.NewImage))
		}
		if len(params.EnvVars) > 0 {
			diffLines = append(diffLines, fmt.Sprintf("  + env overrides: %v", params.EnvVars))
		}
		if len(params.ResourceLimits) > 0 {
			diffLines = append(diffLines, fmt.Sprintf("  + resource limits: %v", params.ResourceLimits))
		}
		return fmt.Sprintf("[dry-run] UPDATE Deployment %s/%s\ndiff:\n%s", params.Namespace, params.Name, strings.Join(diffLines, "\n")), nil

	case "create_namespace":
		if _, err := client.Clientset.CoreV1().Namespaces().Get(ctx, params.Name, metav1.GetOptions{}); err == nil {
			return "", fmt.Errorf("namespace %s already exists", params.Name)
		}
		return fmt.Sprintf("[dry-run] CREATE Namespace %s\nserver_dry_run: ok", params.Name), nil

	case "create_ingress":
		if _, err := client.Clientset.CoreV1().Services(params.Namespace).Get(ctx, params.BackendService, metav1.GetOptions{}); err != nil {
			return "", fmt.Errorf("backend service %s/%s not found", params.Namespace, params.BackendService)
		}
		ingPath := params.Path
		if ingPath == "" {
			ingPath = "/"
		}
		bPort := params.BackendPort
		if bPort <= 0 {
			bPort = 80
		}
		return fmt.Sprintf("[dry-run] CREATE Ingress %s/%s\n  host: %s\n  path: %s\n  backend: %s:%d\nserver_dry_run: ok", params.Namespace, params.Name, params.Host, ingPath, params.BackendService, bPort), nil

	case "create_hpa":
		if _, err := client.Clientset.AppsV1().Deployments(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{}); err != nil {
			return "", fmt.Errorf("deployment %s/%s not found for HPA target", params.Namespace, params.Name)
		}
		minR := params.MinReplicas
		if minR <= 0 {
			minR = 1
		}
		return fmt.Sprintf("[dry-run] CREATE HPA for Deployment %s/%s\n  minReplicas: %d\n  maxReplicas: %d\n  targetCPU: %d%%\nserver_dry_run: ok", params.Namespace, params.Name, minR, params.MaxReplicas, params.TargetCPU), nil

	case "create_pvc":
		sc := params.StorageClass
		if sc == "" {
			sc = "(default)"
		}
		am := params.AccessModes
		if len(am) == 0 {
			am = []string{"ReadWriteOnce"}
		}
		return fmt.Sprintf("[dry-run] CREATE PVC %s/%s\n  storageClassName: %s\n  accessModes: %v\n  resources.requests.storage: %s\nserver_dry_run: ok", params.Namespace, params.Name, sc, am, params.StorageSize), nil

	case "apply_yaml":
		return s.dryRunApplyYAML(ctx, clusterID, params)

		default:
		return "", fmt.Errorf("unsupported action: %s", params.Action)
	}
}

func buildDeploymentObject(params StagedActionParams) *appsv1.Deployment {
	containerPorts := make([]corev1.ContainerPort, 0, len(params.Ports))
	for _, p := range params.Ports {
		containerPorts = append(containerPorts, corev1.ContainerPort{ContainerPort: p})
	}
	volumes := make([]corev1.Volume, 0, len(params.HostPathMounts))
	volumeMounts := make([]corev1.VolumeMount, 0, len(params.HostPathMounts))
	for i, m := range params.HostPathMounts {
		volName := strings.TrimSpace(m.Name)
		if volName == "" {
			volName = fmt.Sprintf("hostpath-%d", i)
		}
		hpType := corev1.HostPathDirectoryOrCreate
		switch strings.TrimSpace(m.Type) {
		case "Directory":
			hpType = corev1.HostPathDirectory
		case "File":
			hpType = corev1.HostPathFile
		case "FileOrCreate":
			hpType = corev1.HostPathFileOrCreate
		case "DirectoryOrCreate", "":
			hpType = corev1.HostPathDirectoryOrCreate
		}
		t := hpType
		volumes = append(volumes, corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{Path: m.HostPath, Type: &t},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volName,
			MountPath: m.MountPath,
			ReadOnly:  m.ReadOnly,
		})
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: params.Name, Namespace: params.Namespace,
			Labels: map[string]string{"app": params.Name},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &params.Replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": params.Name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": params.Name}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:         params.Name,
						Image:        params.Image,
						Ports:        containerPorts,
						VolumeMounts: volumeMounts,
					}},
					Volumes: volumes,
				},
			},
		},
	}
}

func buildServiceObject(params StagedActionParams) *corev1.Service {
	st := params.ServiceType
	if params.NodePort > 0 {
		st = string(corev1.ServiceTypeNodePort)
	}
	if st == "" {
		st = string(corev1.ServiceTypeClusterIP)
	}
	tp := params.TargetPort
	if tp <= 0 {
		tp = params.Port
	}
	port := corev1.ServicePort{
		Name: "http", Port: params.Port, TargetPort: intstr.FromInt(int(tp)), Protocol: corev1.ProtocolTCP,
	}
	if params.NodePort > 0 && st == string(corev1.ServiceTypeNodePort) {
		port.NodePort = params.NodePort
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: params.Name, Namespace: params.Namespace},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceType(st),
			Selector: params.Selector,
			Ports:    []corev1.ServicePort{port},
		},
	}
}

func (s *Service) ExecuteStagedAction(ctx context.Context, action *model.AgentAction) (*ExecuteResult, error) {
	var params StagedActionParams
	if err := json.Unmarshal([]byte(action.Parameters), &params); err != nil {
		return nil, fmt.Errorf("invalid staged parameters: %w", err)
	}

	switch params.Action {
	case "create_deployment":
		return s.ExecuteCreateDeployment(ctx, action.ClusterID, params)
	case "create_service":
		return s.ExecuteCreateService(ctx, action.ClusterID, params.Namespace, params.Name, params.ServiceType, params.Selector, params.Port, params.TargetPort, params.NodePort)
	case "delete_deployment":
		return s.ExecuteDeleteDeployment(ctx, action.ClusterID, params.Namespace, params.Name)
	case "delete_service":
		return s.ExecuteDeleteService(ctx, action.ClusterID, params.Namespace, params.Name)
	case "delete_pod":
		return s.ExecuteDeletePod(ctx, action.ClusterID, params.Namespace, params.Name)
	case "scale_deployment":
		return s.ExecuteScaleDeployment(ctx, action.ClusterID, params.Namespace, params.Name, params.Replicas)
	case "create_configmap":
		return s.ExecuteCreateConfigMap(ctx, action.ClusterID, params)
	case "create_secret":
		return s.ExecuteCreateSecret(ctx, action.ClusterID, params)
	case "update_deployment":
		return s.ExecuteUpdateDeployment(ctx, action.ClusterID, params)
	case "create_namespace":
		return s.ExecuteCreateNamespace(ctx, action.ClusterID, params.Name)
	case "create_ingress":
		return s.ExecuteCreateIngress(ctx, action.ClusterID, params)
	case "create_hpa":
		return s.ExecuteCreateHPA(ctx, action.ClusterID, params)
	case "create_pvc":
		return s.ExecuteCreatePVC(ctx, action.ClusterID, params)
	case "apply_yaml":
		return s.ExecuteApplyYAML(ctx, action.ClusterID, params)

		default:
		return nil, fmt.Errorf("unsupported action: %s", params.Action)
	}
}
