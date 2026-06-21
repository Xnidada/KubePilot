package workload

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type KubectlExecutor interface {
	ExecuteKubectl(ctx context.Context, clusterID uint, args []string) (bool, string, string, error)
	ExecuteKubectlApply(ctx context.Context, clusterID uint, yamlContent string) (bool, string, string, error)
	ExecuteKubectlDelete(ctx context.Context, clusterID uint, yamlContent string) (bool, string, string, error)
}

type Handler struct {
	kubectlExecutor KubectlExecutor
}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) SetKubectlExecutor(executor KubectlExecutor) {
	h.kubectlExecutor = executor
}

// Deployment handlers
func (h *Handler) ListDeployments(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Query("ns")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	var deployments *appsv1.DeploymentList
	if namespace == "" {
		// Query all namespaces
		deployments, err = client.Clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	} else {
		deployments, err = client.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type DeploymentInfo struct {
		Name      string   `json:"name"`
		Namespace string   `json:"namespace"`
		Status    string   `json:"status"`
		Ready     string   `json:"ready"`
		UpToDate  int32    `json:"up_to_date"`
		Available int32    `json:"available"`
		Age       string   `json:"age"`
		Images    []string `json:"images"`
	}

	result := make([]DeploymentInfo, 0, len(deployments.Items))
	for _, d := range deployments.Items {
		images := make([]string, 0)
		for _, c := range d.Spec.Template.Spec.Containers {
			images = append(images, c.Image)
		}
		var replicas int32 = 0
		if d.Spec.Replicas != nil {
			replicas = *d.Spec.Replicas
		}

		// 检查是否处于Terminating状态
		status := "Active"
		if d.DeletionTimestamp != nil {
			status = "Terminating"
		} else if d.Status.ReadyReplicas < replicas {
			status = "Updating"
		}

		result = append(result, DeploymentInfo{
			Name:      d.Name,
			Namespace: d.Namespace,
			Status:    status,
			Ready:     strconv.Itoa(int(d.Status.ReadyReplicas)) + "/" + strconv.Itoa(int(replicas)),
			UpToDate:  d.Status.UpdatedReplicas,
			Available: d.Status.AvailableReplicas,
			Age:       timeSince(d.CreationTimestamp.Time),
			Images:    images,
		})
	}

	response.Success(c, result)
}

func (h *Handler) GetDeployment(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	deployment, err := client.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "deployment not found")
		return
	}

	// 返回完整的配置详情，用于编辑
	type ContainerDetail struct {
		Name            string               `json:"name"`
		Image           string               `json:"image"`
		ImagePullPolicy string               `json:"image_pull_policy"`
		Command         []string             `json:"command,omitempty"`
		Args            []string             `json:"args,omitempty"`
		Resources       *ResourceConfig      `json:"resources,omitempty"`
		Ports           []ContainerPort      `json:"ports,omitempty"`
		Env             []EnvVar             `json:"env,omitempty"`
		VolumeMounts    []VolumeMount        `json:"volume_mounts,omitempty"`
		LivenessProbe   *ProbeConfig         `json:"liveness_probe,omitempty"`
		ReadinessProbe  *ProbeConfig         `json:"readiness_probe,omitempty"`
	}

	type DeploymentDetail struct {
		Name        string            `json:"name"`
		Namespace   string            `json:"namespace"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
		Replicas    int32             `json:"replicas"`
		MaxSurge    *int              `json:"max_surge,omitempty"`
		MaxUnavail  *int              `json:"max_unavailable,omitempty"`
		Containers  []ContainerDetail `json:"containers"`
		Volumes     []VolumeConfig    `json:"volumes,omitempty"`
		NodeSelector map[string]string `json:"node_selector,omitempty"`
		Tolerations []Toleration      `json:"tolerations,omitempty"`
		ServiceAccountName string     `json:"service_account_name,omitempty"`
		DNSPolicy   string            `json:"dns_policy,omitempty"`
		HostNetwork bool              `json:"host_network,omitempty"`
		RestartPolicy string          `json:"restart_policy,omitempty"`
		TerminationGracePeriod *int64 `json:"termination_grace_period,omitempty"`
	}

	detail := DeploymentDetail{
		Name:        deployment.Name,
		Namespace:   deployment.Namespace,
		Labels:      deployment.Labels,
		Annotations: deployment.Annotations,
		NodeSelector: deployment.Spec.Template.Spec.NodeSelector,
		ServiceAccountName: deployment.Spec.Template.Spec.ServiceAccountName,
		DNSPolicy:   string(deployment.Spec.Template.Spec.DNSPolicy),
		HostNetwork: deployment.Spec.Template.Spec.HostNetwork,
		RestartPolicy: string(deployment.Spec.Template.Spec.RestartPolicy),
		TerminationGracePeriod: deployment.Spec.Template.Spec.TerminationGracePeriodSeconds,
	}

	if deployment.Spec.Replicas != nil {
		detail.Replicas = *deployment.Spec.Replicas
	}

	// 解析滚动更新策略
	if deployment.Spec.Strategy.RollingUpdate != nil {
		if deployment.Spec.Strategy.RollingUpdate.MaxSurge != nil {
			val, _ := intstr.GetScaledValueFromIntOrPercent(deployment.Spec.Strategy.RollingUpdate.MaxSurge, 100, true)
			detail.MaxSurge = &val
		}
		if deployment.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
			val, _ := intstr.GetScaledValueFromIntOrPercent(deployment.Spec.Strategy.RollingUpdate.MaxUnavailable, 100, true)
			detail.MaxUnavail = &val
		}
	}

	// 解析容器配置
	for _, c := range deployment.Spec.Template.Spec.Containers {
		container := ContainerDetail{
			Name:            c.Name,
			Image:           c.Image,
			ImagePullPolicy: string(c.ImagePullPolicy),
			Command:         c.Command,
			Args:            c.Args,
		}

		// 资源配额
		if len(c.Resources.Requests) > 0 || len(c.Resources.Limits) > 0 {
			container.Resources = &ResourceConfig{}
			if cpu, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				container.Resources.CPURequest = cpu.String()
			}
			if cpu, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
				container.Resources.CPULimit = cpu.String()
			}
			if mem, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				container.Resources.MemoryRequest = mem.String()
			}
			if mem, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
				container.Resources.MemoryLimit = mem.String()
			}
		}

		// 端口
		for _, p := range c.Ports {
			container.Ports = append(container.Ports, ContainerPort{
				Name:          p.Name,
				ContainerPort: p.ContainerPort,
				Protocol:      string(p.Protocol),
			})
		}

		// 环境变量
		for _, e := range c.Env {
			envVar := EnvVar{Name: e.Name, Value: e.Value}
			if e.ValueFrom != nil {
				if e.ValueFrom.ConfigMapKeyRef != nil {
					envVar.ValueFrom = &EnvVarSource{
						Type:          "configmap",
						ConfigMapName: e.ValueFrom.ConfigMapKeyRef.Name,
						ConfigMapKey:  e.ValueFrom.ConfigMapKeyRef.Key,
					}
				} else if e.ValueFrom.SecretKeyRef != nil {
					envVar.ValueFrom = &EnvVarSource{
						Type:       "secret",
						SecretName: e.ValueFrom.SecretKeyRef.Name,
						SecretKey:  e.ValueFrom.SecretKeyRef.Key,
					}
				} else if e.ValueFrom.FieldRef != nil {
					envVar.ValueFrom = &EnvVarSource{
						Type:      "field",
						FieldPath: e.ValueFrom.FieldRef.FieldPath,
					}
				}
			}
			container.Env = append(container.Env, envVar)
		}

		// 挂载卷
		for _, m := range c.VolumeMounts {
			container.VolumeMounts = append(container.VolumeMounts, VolumeMount{
				Name:      m.Name,
				MountPath: m.MountPath,
				SubPath:   m.SubPath,
				ReadOnly:  m.ReadOnly,
			})
		}

		// 健康检查
		if c.LivenessProbe != nil {
			container.LivenessProbe = parseProbe(c.LivenessProbe)
		}
		if c.ReadinessProbe != nil {
			container.ReadinessProbe = parseProbe(c.ReadinessProbe)
		}

		detail.Containers = append(detail.Containers, container)
	}

	// 解析卷配置
	for _, v := range deployment.Spec.Template.Spec.Volumes {
		volConfig := VolumeConfig{Name: v.Name}
		if v.HostPath != nil {
			volConfig.Type = "hostpath"
			volConfig.HostPath = &HostPathConfig{Path: v.HostPath.Path}
		} else if v.PersistentVolumeClaim != nil {
			volConfig.Type = "pvc"
			volConfig.PVCName = v.PersistentVolumeClaim.ClaimName
		} else if v.ConfigMap != nil {
			volConfig.Type = "configmap"
			volConfig.ConfigMap = v.ConfigMap.Name
		} else if v.Secret != nil {
			volConfig.Type = "secret"
			volConfig.Secret = v.Secret.SecretName
		} else if v.EmptyDir != nil {
			volConfig.Type = "emptydir"
			ed := &EmptyDirConfig{}
			if v.EmptyDir.Medium == corev1.StorageMediumMemory {
				ed.Medium = "memory"
			}
			if v.EmptyDir.SizeLimit != nil {
				ed.SizeLimit = v.EmptyDir.SizeLimit.String()
			}
			volConfig.EmptyDir = ed
		}
		detail.Volumes = append(detail.Volumes, volConfig)
	}

	// 解析污点容忍
	for _, t := range deployment.Spec.Template.Spec.Tolerations {
		detail.Tolerations = append(detail.Tolerations, Toleration{
			Key:      t.Key,
			Operator: string(t.Operator),
			Value:    t.Value,
			Effect:   string(t.Effect),
		})
	}

	response.Success(c, detail)
}

// parseProbe 解析探针配置
func parseProbe(probe *corev1.Probe) *ProbeConfig {
	config := &ProbeConfig{
		InitialDelaySecs: probe.InitialDelaySeconds,
		PeriodSecs:       probe.PeriodSeconds,
		TimeoutSecs:      probe.TimeoutSeconds,
		SuccessThres:     probe.SuccessThreshold,
		FailureThres:     probe.FailureThreshold,
	}

	if probe.HTTPGet != nil {
		config.ProbeType = "http"
		config.HTTPGet = &HTTPGetProbe{
			Path:   probe.HTTPGet.Path,
			Port:   probe.HTTPGet.Port.IntVal,
			Scheme: string(probe.HTTPGet.Scheme),
		}
	} else if probe.TCPSocket != nil {
		config.ProbeType = "tcp"
		config.TCPSocket = &TCPSocket{
			Port: probe.TCPSocket.Port.IntVal,
		}
	} else if probe.Exec != nil {
		config.ProbeType = "exec"
		config.Exec = &ExecProbe{
			Command: probe.Exec.Command,
		}
	}

	return config
}

func (h *Handler) ScaleDeployment(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	var req struct {
		Replicas int32 `json:"replicas"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	deployment, err := client.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "deployment not found")
		return
	}

	deployment.Spec.Replicas = &req.Replicas
	_, err = client.Clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "deployment scaled successfully", nil)
}

func (h *Handler) UpdateDeployment(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	// 使用与创建相同的请求结构
	var req UpdateDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	deployment, err := client.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "deployment not found")
		return
	}

	// 更新副本数
	if req.Replicas != nil {
		deployment.Spec.Replicas = req.Replicas
	}

	// 更新标签
	if req.Labels != nil {
		labels := deployment.Labels
		if labels == nil {
			labels = make(map[string]string)
		}
		for k, v := range req.Labels {
			labels[k] = v
		}
		deployment.Labels = labels
		deployment.Spec.Template.Labels = labels
	}

	// 更新注解
	if req.Annotations != nil {
		deployment.Annotations = req.Annotations
		deployment.Spec.Template.Annotations = req.Annotations
	}

	// 更新容器配置
	if len(req.Containers) > 0 {
		containers := buildContainers(req.Containers)
		deployment.Spec.Template.Spec.Containers = containers
	}

	// 更新初始化容器
	if req.InitContainers != nil {
		initContainers := buildContainers(req.InitContainers)
		deployment.Spec.Template.Spec.InitContainers = initContainers
	}

	// 更新卷
	if req.Volumes != nil {
		volumes := buildVolumes(req.Volumes)
		deployment.Spec.Template.Spec.Volumes = volumes
	}

	// 更新滚动更新策略
	if req.MaxSurge != nil || req.MaxUnavail != nil {
		maxSurge := intstr.FromInt(1)
		maxUnavail := intstr.FromInt(0)
		if req.MaxSurge != nil {
			maxSurge = intstr.FromInt(*req.MaxSurge)
		}
		if req.MaxUnavail != nil {
			maxUnavail = intstr.FromInt(*req.MaxUnavail)
		}
		deployment.Spec.Strategy = appsv1.DeploymentStrategy{
			Type: appsv1.RollingUpdateDeploymentStrategyType,
			RollingUpdate: &appsv1.RollingUpdateDeployment{
				MaxSurge:       &maxSurge,
				MaxUnavailable: &maxUnavail,
			},
		}
	}

	// 更新调度配置
	if req.Scheduling != nil {
		deployment.Spec.Template.Spec.NodeSelector = req.Scheduling.NodeSelector

		if req.Scheduling.Tolerations != nil {
			tolerations := make([]corev1.Toleration, 0)
			for _, t := range req.Scheduling.Tolerations {
				tolerations = append(tolerations, corev1.Toleration{
					Key:      t.Key,
					Operator: corev1.TolerationOperator(t.Operator),
					Value:    t.Value,
					Effect:   corev1.TaintEffect(t.Effect),
				})
			}
			deployment.Spec.Template.Spec.Tolerations = tolerations
		}
	}

	// 更新高级配置
	if req.Advanced != nil {
		if req.Advanced.ServiceAccountName != "" {
			deployment.Spec.Template.Spec.ServiceAccountName = req.Advanced.ServiceAccountName
		}
		if req.Advanced.TerminationGracePeriodSecs != nil {
			deployment.Spec.Template.Spec.TerminationGracePeriodSeconds = req.Advanced.TerminationGracePeriodSecs
		}
		if req.Advanced.DNSPolicy != "" {
			deployment.Spec.Template.Spec.DNSPolicy = corev1.DNSPolicy(req.Advanced.DNSPolicy)
		}
		if req.Advanced.HostNetwork {
			deployment.Spec.Template.Spec.HostNetwork = true
		}
		if req.Advanced.RestartPolicy != "" {
			deployment.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicy(req.Advanced.RestartPolicy)
		}
		if len(req.Advanced.ImagePullSecrets) > 0 {
			secrets := make([]corev1.LocalObjectReference, 0)
			for _, s := range req.Advanced.ImagePullSecrets {
				secrets = append(secrets, corev1.LocalObjectReference{Name: s})
			}
			deployment.Spec.Template.Spec.ImagePullSecrets = secrets
		}
		if req.Advanced.PodSecurityContext != nil {
			deployment.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
				RunAsUser:    req.Advanced.PodSecurityContext.RunAsUser,
				RunAsGroup:   req.Advanced.PodSecurityContext.RunAsGroup,
				RunAsNonRoot: req.Advanced.PodSecurityContext.RunAsNonRoot,
				FSGroup:      req.Advanced.PodSecurityContext.FSGroup,
			}
		}
	}

	_, err = client.Clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 更新关联的Service
	if req.Service != nil {
		if req.Service.Name != "" {
			// 更新现有Service
			service, err := client.Clientset.CoreV1().Services(namespace).Get(ctx, req.Service.Name, metav1.GetOptions{})
			if err == nil {
				if req.Service.Type != "" {
					service.Spec.Type = corev1.ServiceType(req.Service.Type)
				}
				if req.Service.Ports != nil {
					svcPorts := make([]corev1.ServicePort, 0)
					for i, p := range req.Service.Ports {
						portName := p.Name
						if portName == "" {
							portName = "port-" + strconv.Itoa(i)
						}
						protocol := corev1.ProtocolTCP
						if p.Protocol == "UDP" {
							protocol = corev1.ProtocolUDP
						}
						svcPort := corev1.ServicePort{
							Name:       portName,
							Port:       p.Port,
							TargetPort: intstr.FromInt(int(p.TargetPort)),
							Protocol:   protocol,
						}
						if p.NodePort > 0 {
							svcPort.NodePort = p.NodePort
						}
						svcPorts = append(svcPorts, svcPort)
					}
					service.Spec.Ports = svcPorts
				}
				client.Clientset.CoreV1().Services(namespace).Update(ctx, service, metav1.UpdateOptions{})
			}
		} else if req.Service.Create {
			// 创建新Service
			svcName := name
			svcPorts := make([]corev1.ServicePort, 0)
			for i, p := range req.Service.Ports {
				portName := p.Name
				if portName == "" {
					portName = "port-" + strconv.Itoa(i)
				}
				protocol := corev1.ProtocolTCP
				if p.Protocol == "UDP" {
					protocol = corev1.ProtocolUDP
				}
				svcPort := corev1.ServicePort{
					Name:       portName,
					Port:       p.Port,
					TargetPort: intstr.FromInt(int(p.TargetPort)),
					Protocol:   protocol,
				}
				if p.NodePort > 0 {
					svcPort.NodePort = p.NodePort
				}
				svcPorts = append(svcPorts, svcPort)
			}

			serviceType := corev1.ServiceTypeClusterIP
			if req.Service.Type != "" {
				serviceType = corev1.ServiceType(req.Service.Type)
			}

			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: namespace,
					Labels:    deployment.Labels,
				},
				Spec: corev1.ServiceSpec{
					Type:     serviceType,
					Selector: deployment.Spec.Selector.MatchLabels,
					Ports:    svcPorts,
				},
			}
			client.Clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
		}
	}

	response.SuccessWithMessage(c, "deployment updated", nil)
}

// UpdateDeploymentRequest 更新Deployment请求
type UpdateDeploymentRequest struct {
	Replicas       *int32                `json:"replicas"`
	Labels         map[string]string     `json:"labels"`
	Annotations    map[string]string     `json:"annotations"`
	MaxSurge       *int                  `json:"max_surge"`
	MaxUnavail     *int                  `json:"max_unavailable"`
	Containers     []ContainerConfig     `json:"containers"`
	InitContainers []ContainerConfig     `json:"init_containers"`
	Volumes        []VolumeConfig        `json:"volumes"`
	Scheduling     *SchedulingConfig     `json:"scheduling"`
	Advanced       *AdvancedConfig       `json:"advanced"`
	Service        *UpdateServiceConfig  `json:"service"`
}

// UpdateServiceConfig 更新Service配置
type UpdateServiceConfig struct {
	Name   string           `json:"name"`
	Create bool             `json:"create"`
	Type   string           `json:"type"`
	Ports  []ServicePort    `json:"ports"`
}

func (h *Handler) DeleteDeployment(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	err = client.Clientset.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "deployment deleted", nil)
}

// GetDeploymentHistory 获取Deployment修订历史
func (h *Handler) GetDeploymentHistory(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()

	// 获取Deployment
	deployment, err := client.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "deployment not found")
		return
	}

	// 获取关联的ReplicaSets
	selector := metav1.FormatLabelSelector(deployment.Spec.Selector)
	replicaSets, err := client.Clientset.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type RevisionInfo struct {
		Revision    int    `json:"revision"`
		Replicas    int32  `json:"replicas"`
		CreateTime  string `json:"create_time"`
		Images      string `json:"images"`
		Annotations map[string]string `json:"annotations,omitempty"`
	}

	revisions := make([]RevisionInfo, 0)
	for _, rs := range replicaSets.Items {
		revision := 0
		if v, ok := rs.Annotations["deployment.kubernetes.io/revision"]; ok {
			revision, _ = strconv.Atoi(v)
		}

		images := ""
		for i, c := range rs.Spec.Template.Spec.Containers {
			if i > 0 {
				images += ", "
			}
			images += c.Image
		}

		revisions = append(revisions, RevisionInfo{
			Revision:    revision,
			Replicas:    *rs.Spec.Replicas,
			CreateTime:  rs.CreationTimestamp.Format("2006-01-02 15:04:05"),
			Images:      images,
			Annotations: rs.Annotations,
		})
	}

	// 按修订版本排序
	for i := 0; i < len(revisions)-1; i++ {
		for j := i + 1; j < len(revisions); j++ {
			if revisions[i].Revision < revisions[j].Revision {
				revisions[i], revisions[j] = revisions[j], revisions[i]
			}
		}
	}

	result := map[string]interface{}{
		"deployment": name,
		"namespace":  namespace,
		"current_revision": deployment.Annotations["deployment.kubernetes.io/revision"],
		"revisions":  revisions,
	}

	response.Success(c, result)
}

// RollbackDeployment 回滚Deployment到指定版本
func (h *Handler) RollbackDeployment(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	var req struct {
		Revision int `json:"revision"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()

	// 获取Deployment
	deployment, err := client.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "deployment not found")
		return
	}

	// 获取目标Revision的ReplicaSet
	selector := metav1.FormatLabelSelector(deployment.Spec.Selector)
	replicaSets, err := client.Clientset.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	var targetRS *appsv1.ReplicaSet
	for _, rs := range replicaSets.Items {
		revision := 0
		if v, ok := rs.Annotations["deployment.kubernetes.io/revision"]; ok {
			revision, _ = strconv.Atoi(v)
		}
		if revision == req.Revision {
			targetRS = &rs
			break
		}
	}

	if targetRS == nil {
		response.NotFound(c, "revision not found")
		return
	}

	// 执行回滚 - 更新Deployment的Pod模板为目标Revision
	deployment.Spec.Template = targetRS.Spec.Template
	_, err = client.Clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, fmt.Sprintf("rollback to revision %d successful", req.Revision), nil)
}

func (h *Handler) CreateDeployment(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		Namespace string `json:"namespace" binding:"required"`
		Name      string `json:"name" binding:"required"`
		Image     string `json:"image" binding:"required"`
		Replicas  int32  `json:"replicas"`
		Ports     []struct {
			ContainerPort int32 `json:"containerPort"`
		} `json:"ports"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if req.Replicas == 0 {
		req.Replicas = 1
	}

	ports := make([]corev1.ContainerPort, 0)
	for _, p := range req.Ports {
		ports = append(ports, corev1.ContainerPort{
			ContainerPort: p.ContainerPort,
		})
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &req.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": req.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": req.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  req.Name,
							Image: req.Image,
							Ports: ports,
						},
					},
				},
			},
		},
	}

	ctx := context.Background()
	result, err := client.Clientset.AppsV1().Deployments(req.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

// Pod handlers
func (h *Handler) ListPods(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Query("ns")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	var pods *corev1.PodList
	if namespace == "" {
		// Query all namespaces
		pods, err = client.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	} else {
		pods, err = client.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type PodInfo struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Status    string `json:"status"`
		Ready     string `json:"ready"`
		Restarts  int32  `json:"restarts"`
		Age       string `json:"age"`
		Node      string `json:"node"`
		IP        string `json:"ip"`
	}

	result := make([]PodInfo, 0, len(pods.Items))
	for _, p := range pods.Items {
		readyCount := 0
		totalCount := len(p.Spec.Containers)
		var restarts int32
		for _, cs := range p.Status.ContainerStatuses {
			if cs.Ready {
				readyCount++
			}
			restarts += cs.RestartCount
		}

		// 检查是否处于Terminating状态
		status := string(p.Status.Phase)
		if p.DeletionTimestamp != nil {
			status = "Terminating"
		}

		result = append(result, PodInfo{
			Name:      p.Name,
			Namespace: p.Namespace,
			Status:    status,
			Ready:     strconv.Itoa(readyCount) + "/" + strconv.Itoa(totalCount),
			Restarts:  restarts,
			Age:       timeSince(p.CreationTimestamp.Time),
			Node:      p.Spec.NodeName,
			IP:        p.Status.PodIP,
		})
	}

	response.Success(c, result)
}

func (h *Handler) GetPod(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	pod, err := client.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "pod not found")
		return
	}

	response.Success(c, pod)
}

func (h *Handler) GetPodLogs(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")
	tailLines := c.DefaultQuery("tail", "100")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	lines, _ := strconv.ParseInt(tailLines, 10, 64)
	opts := &metav1.ListOptions{
		Limit: lines,
	}

	_ = opts
	logs, err := client.Clientset.CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{
		TailLines: &lines,
	}).DoRaw(ctx)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.Data(200, "text/plain; charset=utf-8", logs)
}

func (h *Handler) DeletePod(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	err = client.Clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "pod deleted", nil)
}

func (h *Handler) CreatePod(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		Namespace string `json:"namespace" binding:"required"`
		Name      string `json:"name" binding:"required"`
		Image     string `json:"image" binding:"required"`
		Ports     []struct {
			ContainerPort int32 `json:"containerPort"`
		} `json:"ports"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ports := make([]corev1.ContainerPort, 0)
	for _, p := range req.Ports {
		ports = append(ports, corev1.ContainerPort{
			ContainerPort: p.ContainerPort,
		})
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
			Labels: map[string]string{
				"app": req.Name,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  req.Name,
					Image: req.Image,
					Ports: ports,
				},
			},
		},
	}

	ctx := context.Background()
	result, err := client.Clientset.CoreV1().Pods(req.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

// Service handlers
func (h *Handler) ListServices(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Query("ns")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	var services *corev1.ServiceList
	if namespace == "" {
		// Query all namespaces
		services, err = client.Clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	} else {
		services, err = client.Clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type ServiceInfo struct {
		Name       string `json:"name"`
		Namespace  string `json:"namespace"`
		Status     string `json:"status"`
		Type       string `json:"type"`
		ClusterIP  string `json:"cluster_ip"`
		Ports      string `json:"ports"`
		Age        string `json:"age"`
	}

	result := make([]ServiceInfo, 0, len(services.Items))
	for _, s := range services.Items {
		ports := ""
		for i, p := range s.Spec.Ports {
			if i > 0 {
				ports += ", "
			}
			ports += strconv.Itoa(int(p.Port))
			if p.NodePort > 0 {
				ports += ":" + strconv.Itoa(int(p.NodePort))
			}
		}

		// 检查是否处于Terminating状态
		status := "Active"
		if s.DeletionTimestamp != nil {
			status = "Terminating"
		}

		result = append(result, ServiceInfo{
			Name:      s.Name,
			Namespace: s.Namespace,
			Status:    status,
			Type:      string(s.Spec.Type),
			ClusterIP: s.Spec.ClusterIP,
			Ports:     ports,
			Age:       timeSince(s.CreationTimestamp.Time),
		})
	}

	response.Success(c, result)
}

func (h *Handler) GetService(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	service, err := client.Clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "service not found")
		return
	}

	type ServicePortInfo struct {
		Name       string `json:"name"`
		Port       int32  `json:"port"`
		TargetPort int32  `json:"target_port"`
		NodePort   int32  `json:"node_port,omitempty"`
		Protocol   string `json:"protocol"`
	}

	ports := make([]ServicePortInfo, 0)
	for _, p := range service.Spec.Ports {
		ports = append(ports, ServicePortInfo{
			Name:       p.Name,
			Port:       p.Port,
			TargetPort: p.TargetPort.IntVal,
			NodePort:   p.NodePort,
			Protocol:   string(p.Protocol),
		})
	}

	result := map[string]interface{}{
		"name":       service.Name,
		"namespace":  service.Namespace,
		"type":       string(service.Spec.Type),
		"cluster_ip": service.Spec.ClusterIP,
		"selector":   service.Spec.Selector,
		"ports":      ports,
	}

	response.Success(c, result)
}

// GetDeploymentServices 获取Deployment关联的Services
func (h *Handler) GetDeploymentServices(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	deployName := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()

	// 获取Deployment的标签
	deployment, err := client.Clientset.AppsV1().Deployments(namespace).Get(ctx, deployName, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "deployment not found")
		return
	}

	// 查找selector匹配的Services
	selectorLabels := deployment.Spec.Selector.MatchLabels
	if len(selectorLabels) == 0 {
		response.Success(c, []interface{}{})
		return
	}

	// 构建label selector字符串
	selectorParts := make([]string, 0)
	for k, v := range selectorLabels {
		selectorParts = append(selectorParts, k+"="+v)
	}
	selector := strings.Join(selectorParts, ",")

	services, err := client.Clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type ServicePortInfo struct {
		Name       string `json:"name"`
		Port       int32  `json:"port"`
		TargetPort int32  `json:"target_port"`
		NodePort   int32  `json:"node_port,omitempty"`
		Protocol   string `json:"protocol"`
	}

	type ServiceInfo struct {
		Name       string            `json:"name"`
		Namespace  string            `json:"namespace"`
		Type       string            `json:"type"`
		ClusterIP  string            `json:"cluster_ip"`
		Selector   map[string]string `json:"selector"`
		Ports      []ServicePortInfo `json:"ports"`
	}

	result := make([]ServiceInfo, 0)
	for _, svc := range services.Items {
		ports := make([]ServicePortInfo, 0)
		for _, p := range svc.Spec.Ports {
			ports = append(ports, ServicePortInfo{
				Name:       p.Name,
				Port:       p.Port,
				TargetPort: p.TargetPort.IntVal,
				NodePort:   p.NodePort,
				Protocol:   string(p.Protocol),
			})
		}

		result = append(result, ServiceInfo{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Type:      string(svc.Spec.Type),
			ClusterIP: svc.Spec.ClusterIP,
			Selector:  svc.Spec.Selector,
			Ports:     ports,
		})
	}

	response.Success(c, result)
}

func (h *Handler) CreateService(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		Namespace string `json:"namespace" binding:"required"`
		Name      string `json:"name" binding:"required"`
		Type      string `json:"type"`
		Selector  map[string]string `json:"selector" binding:"required"`
		Ports     []struct {
			Port       int32  `json:"port"`
			TargetPort int32  `json:"targetPort"`
			Protocol   string `json:"protocol"`
		} `json:"ports" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if req.Type == "" {
		req.Type = "ClusterIP"
	}

	svcPorts := make([]corev1.ServicePort, 0)
	for i, p := range req.Ports {
		protocol := corev1.ProtocolTCP
		if p.Protocol == "UDP" {
			protocol = corev1.ProtocolUDP
		}
		svcPorts = append(svcPorts, corev1.ServicePort{
			Name:       "port-" + strconv.Itoa(i),
			Port:       p.Port,
			TargetPort: intstr.FromInt(int(p.TargetPort)),
			Protocol:   protocol,
		})
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceType(req.Type),
			Selector: req.Selector,
			Ports:    svcPorts,
		},
	}

	ctx := context.Background()
	result, err := client.Clientset.CoreV1().Services(req.Namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

func (h *Handler) UpdateService(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	var req struct {
		Type   string `json:"type"`
		Ports  []struct {
			Name       string `json:"name"`
			Port       int32  `json:"port"`
			TargetPort int32  `json:"targetPort"`
			NodePort   int32  `json:"nodePort"`
			Protocol   string `json:"protocol"`
		} `json:"ports"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	service, err := client.Clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "service not found")
		return
	}

	// 更新类型
	if req.Type != "" {
		service.Spec.Type = corev1.ServiceType(req.Type)
	}

	// 更新端口
	if req.Ports != nil {
		svcPorts := make([]corev1.ServicePort, 0)
		for i, p := range req.Ports {
			portName := p.Name
			if portName == "" {
				portName = "port-" + strconv.Itoa(i)
			}
			protocol := corev1.ProtocolTCP
			if p.Protocol == "UDP" {
				protocol = corev1.ProtocolUDP
			}
			svcPort := corev1.ServicePort{
				Name:       portName,
				Port:       p.Port,
				TargetPort: intstr.FromInt(int(p.TargetPort)),
				Protocol:   protocol,
			}
			if p.NodePort > 0 {
				svcPort.NodePort = p.NodePort
			}
			svcPorts = append(svcPorts, svcPort)
		}
		service.Spec.Ports = svcPorts
	}

	_, err = client.Clientset.CoreV1().Services(namespace).Update(ctx, service, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "service updated", nil)
}

func (h *Handler) DeleteService(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	err = client.Clientset.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "service deleted", nil)
}

// Node handlers
func (h *Handler) ListNodes(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	nodes, err := client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type NodeInfo struct {
		Name        string `json:"name"`
		IP          string `json:"ip"`
		Status      string `json:"status"`
		Roles       string `json:"roles"`
		CPUCapacity string `json:"cpu_capacity"`
		MemCapacity string `json:"mem_capacity"`
		OS          string `json:"os"`
		Kernel      string `json:"kernel"`
		ContainerRT string `json:"container_rt"`
		KubeletVer  string `json:"kubelet_ver"`
	}

	result := make([]NodeInfo, 0, len(nodes.Items))
	for _, n := range nodes.Items {
		ip := ""
		for _, addr := range n.Status.Addresses {
			if addr.Type == "InternalIP" {
				ip = addr.Address
			}
		}

		status := "NotReady"
		for _, cond := range n.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				status = "Ready"
			}
		}

		roles := ""
		for label := range n.Labels {
			if len(label) > 23 && label[:23] == "node-role.kubernetes.io/" {
				if roles != "" {
					roles += ","
				}
				roles += label[23:]
			}
		}
		if roles == "" {
			roles = "<none>"
		}

		result = append(result, NodeInfo{
			Name:        n.Name,
			IP:          ip,
			Status:      status,
			Roles:       roles,
			CPUCapacity: n.Status.Capacity.Cpu().String(),
			MemCapacity: n.Status.Capacity.Memory().String(),
			OS:          n.Status.NodeInfo.OSImage,
			Kernel:      n.Status.NodeInfo.KernelVersion,
			ContainerRT: n.Status.NodeInfo.ContainerRuntimeVersion,
			KubeletVer:  n.Status.NodeInfo.KubeletVersion,
		})
	}

	response.Success(c, result)
}

// GetNode 获取节点详情
func (h *Handler) GetNode(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	node, err := client.Clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "node not found")
		return
	}

	// 获取节点上的 Pod
	pods, _ := client.Clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + name,
	})

	podList := make([]map[string]interface{}, 0)
	if pods != nil {
		for _, pod := range pods.Items {
			podList = append(podList, map[string]interface{}{
				"name":      pod.Name,
				"namespace": pod.Namespace,
				"status":    string(pod.Status.Phase),
			})
		}
	}

	result := map[string]interface{}{
		"name":       node.Name,
		"labels":     node.Labels,
		"annotations": node.Annotations,
		"status":     node.Status.Conditions,
		"capacity":   node.Status.Capacity,
		"allocatable": node.Status.Allocatable,
		"node_info":  node.Status.NodeInfo,
		"pods":       podList,
		"created_at": node.CreationTimestamp.Time,
	}

	response.Success(c, result)
}

// UpdateNode 更新节点（cordon/uncordon/label/taint）
func (h *Handler) UpdateNode(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	name := c.Param("name")

	var req struct {
		Labels      map[string]string `json:"labels"`
		Unschedulable *bool           `json:"unschedulable"` // cordon/uncordon
		Taints      []struct {
			Key    string `json:"key"`
			Value  string `json:"value"`
			Effect string `json:"effect"`
		} `json:"taints"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	node, err := client.Clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "node not found")
		return
	}

	// 更新标签
	if req.Labels != nil {
		if node.Labels == nil {
			node.Labels = make(map[string]string)
		}
		for k, v := range req.Labels {
			node.Labels[k] = v
		}
	}

	// cordon/uncordon
	if req.Unschedulable != nil {
		node.Spec.Unschedulable = *req.Unschedulable
	}

	// 更新 taints
	if req.Taints != nil {
		taints := make([]corev1.Taint, 0)
		for _, t := range req.Taints {
			taints = append(taints, corev1.Taint{
				Key:    t.Key,
				Value:  t.Value,
				Effect: corev1.TaintEffect(t.Effect),
			})
		}
		node.Spec.Taints = taints
	}

	result, err := client.Clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// Namespace handlers
func (h *Handler) ListNamespaces(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	namespaces, err := client.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type NamespaceInfo struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Age    string `json:"age"`
	}

	result := make([]NamespaceInfo, 0, len(namespaces.Items))
	for _, ns := range namespaces.Items {
		// 检查是否处于Terminating状态
		status := string(ns.Status.Phase)
		if ns.DeletionTimestamp != nil {
			status = "Terminating"
		}

		result = append(result, NamespaceInfo{
			Name:   ns.Name,
			Status: status,
			Age:    timeSince(ns.CreationTimestamp.Time),
		})
	}

	response.Success(c, result)
}

// ListNamespaceNames 只返回命名空间名称列表（用于下拉选择）
func (h *Handler) ListNamespaceNames(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	namespaces, err := client.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	result := make([]string, 0, len(namespaces.Items))
	for _, ns := range namespaces.Items {
		result = append(result, ns.Name)
	}

	response.Success(c, result)
}

// Event handlers
func (h *Handler) ListEvents(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.DefaultQuery("ns", "")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	var events *corev1.EventList
	if namespace == "" {
		events, err = client.Clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{Limit: 100})
	} else {
		events, err = client.Clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{Limit: 100})
	}
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type EventInfo struct {
		Type      string `json:"type"`
		Reason    string `json:"reason"`
		Message   string `json:"message"`
		Namespace string `json:"namespace"`
		Object    string `json:"object"`
		Age       string `json:"age"`
	}

	result := make([]EventInfo, 0, len(events.Items))
	for _, e := range events.Items {
		result = append(result, EventInfo{
			Type:      e.Type,
			Reason:    e.Reason,
			Message:   e.Message,
			Namespace: e.Namespace,
			Object:    e.InvolvedObject.Name,
			Age:       timeSince(e.LastTimestamp.Time),
		})
	}

	response.Success(c, result)
}

// PV handlers
func (h *Handler) ListPVs(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	pvs, err := client.Clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type PVInfo struct {
		Name        string `json:"name"`
		Capacity    string `json:"capacity"`
		AccessModes string `json:"access_modes"`
		ReclaimPolicy string `json:"reclaim_policy"`
		Status      string `json:"status"`
		Claim       string `json:"claim"`
		StorageClass string `json:"storage_class"`
		Age         string `json:"age"`
	}

	result := make([]PVInfo, 0, len(pvs.Items))
	for _, pv := range pvs.Items {
		capacity := ""
		if storage, ok := pv.Spec.Capacity[corev1.ResourceStorage]; ok {
			capacity = storage.String()
		}

		accessModes := ""
		for i, mode := range pv.Spec.AccessModes {
			if i > 0 {
				accessModes += ", "
			}
			accessModes += string(mode)
		}

		claim := ""
		if pv.Spec.ClaimRef != nil {
			claim = pv.Spec.ClaimRef.Namespace + "/" + pv.Spec.ClaimRef.Name
		}

		storageClass := ""
		if pv.Spec.StorageClassName != "" {
			storageClass = pv.Spec.StorageClassName
		}

		result = append(result, PVInfo{
			Name:          pv.Name,
			Capacity:      capacity,
			AccessModes:   accessModes,
			ReclaimPolicy: string(pv.Spec.PersistentVolumeReclaimPolicy),
			Status:        string(pv.Status.Phase),
			Claim:         claim,
			StorageClass:  storageClass,
			Age:           timeSince(pv.CreationTimestamp.Time),
		})
	}

	response.Success(c, result)
}

func (h *Handler) GetPV(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	pv, err := client.Clientset.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "pv not found")
		return
	}

	response.Success(c, pv)
}

func (h *Handler) CreatePV(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		Name          string `json:"name" binding:"required"`
		Capacity      string `json:"capacity" binding:"required"`
		AccessModes   []string `json:"access_modes" binding:"required"`
		ReclaimPolicy string `json:"reclaim_policy"`
		StorageClass  string `json:"storage_class"`
		HostPath      string `json:"host_path"`
		NFS           *struct {
			Server string `json:"server"`
			Path   string `json:"path"`
		} `json:"nfs"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if req.ReclaimPolicy == "" {
		req.ReclaimPolicy = "Retain"
	}

	accessModes := make([]corev1.PersistentVolumeAccessMode, 0)
	for _, mode := range req.AccessModes {
		accessModes = append(accessModes, corev1.PersistentVolumeAccessMode(mode))
	}

	capacity, err := resource.ParseQuantity(req.Capacity)
	if err != nil {
		response.BadRequest(c, "invalid capacity: "+err.Error())
		return
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Name,
			Labels: map[string]string{
				"name": req.Name,
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: capacity,
			},
			AccessModes:                   accessModes,
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimPolicy(req.ReclaimPolicy),
			StorageClassName:              req.StorageClass,
		},
	}

	// Set volume source
	if req.HostPath != "" {
		pv.Spec.PersistentVolumeSource = corev1.PersistentVolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: req.HostPath,
			},
		}
	} else if req.NFS != nil {
		pv.Spec.PersistentVolumeSource = corev1.PersistentVolumeSource{
			NFS: &corev1.NFSVolumeSource{
				Server: req.NFS.Server,
				Path:   req.NFS.Path,
			},
		}
	}

	ctx := context.Background()
	result, err := client.Clientset.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

func (h *Handler) DeletePV(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	err = client.Clientset.CoreV1().PersistentVolumes().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "pv deleted", nil)
}

// PVC handlers
func (h *Handler) ListPVCs(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Query("ns")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	var pvcs *corev1.PersistentVolumeClaimList
	if namespace == "" {
		pvcs, err = client.Clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	} else {
		pvcs, err = client.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type PVCInfo struct {
		Name         string `json:"name"`
		Namespace    string `json:"namespace"`
		Status       string `json:"status"`
		Volume       string `json:"volume"`
		Capacity     string `json:"capacity"`
		AccessModes  string `json:"access_modes"`
		StorageClass string `json:"storage_class"`
		Age          string `json:"age"`
	}

	result := make([]PVCInfo, 0, len(pvcs.Items))
	for _, pvc := range pvcs.Items {
		accessModes := ""
		for i, mode := range pvc.Spec.AccessModes {
			if i > 0 {
				accessModes += ", "
			}
			accessModes += string(mode)
		}

		capacity := ""
		if pvc.Status.Capacity != nil {
			if storage, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
				capacity = storage.String()
			}
		}

		storageClass := ""
		if pvc.Spec.StorageClassName != nil {
			storageClass = *pvc.Spec.StorageClassName
		}

		result = append(result, PVCInfo{
			Name:         pvc.Name,
			Namespace:    pvc.Namespace,
			Status:       string(pvc.Status.Phase),
			Volume:       pvc.Spec.VolumeName,
			Capacity:     capacity,
			AccessModes:  accessModes,
			StorageClass: storageClass,
			Age:          timeSince(pvc.CreationTimestamp.Time),
		})
	}

	response.Success(c, result)
}

func (h *Handler) GetPVC(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	pvc, err := client.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "pvc not found")
		return
	}

	response.Success(c, pvc)
}

func (h *Handler) CreatePVC(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		Namespace     string   `json:"namespace" binding:"required"`
		Name          string   `json:"name" binding:"required"`
		Capacity      string   `json:"capacity" binding:"required"`
		AccessModes   []string `json:"access_modes" binding:"required"`
		StorageClass  string   `json:"storage_class"`
		VolumeName    string   `json:"volume_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	accessModes := make([]corev1.PersistentVolumeAccessMode, 0)
	for _, mode := range req.AccessModes {
		accessModes = append(accessModes, corev1.PersistentVolumeAccessMode(mode))
	}

	capacity, err := resource.ParseQuantity(req.Capacity)
	if err != nil {
		response.BadRequest(c, "invalid capacity: "+err.Error())
		return
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: capacity,
				},
			},
			VolumeName: req.VolumeName,
		},
	}

	if req.StorageClass != "" {
		pvc.Spec.StorageClassName = &req.StorageClass
	}

	ctx := context.Background()
	result, err := client.Clientset.CoreV1().PersistentVolumeClaims(req.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

func (h *Handler) DeletePVC(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	err = client.Clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "pvc deleted", nil)
}

// StorageClass handlers
func (h *Handler) ListStorageClasses(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	storageClasses, err := client.Clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type SCInfo struct {
		Name              string `json:"name"`
		Provisioner       string `json:"provisioner"`
		ReclaimPolicy     string `json:"reclaim_policy"`
		VolumeBindingMode string `json:"volume_binding_mode"`
		Age               string `json:"age"`
	}

	result := make([]SCInfo, 0, len(storageClasses.Items))
	for _, sc := range storageClasses.Items {
		volumeBindingMode := ""
		if sc.VolumeBindingMode != nil {
			volumeBindingMode = string(*sc.VolumeBindingMode)
		}

		reclaimPolicy := "Delete"
		if sc.ReclaimPolicy != nil {
			reclaimPolicy = string(*sc.ReclaimPolicy)
		}

		result = append(result, SCInfo{
			Name:              sc.Name,
			Provisioner:       sc.Provisioner,
			ReclaimPolicy:     reclaimPolicy,
			VolumeBindingMode: volumeBindingMode,
			Age:               timeSince(sc.CreationTimestamp.Time),
		})
	}

	response.Success(c, result)
}

// UpdatePV 更新 PersistentVolume
func (h *Handler) UpdatePV(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	name := c.Param("name")

	var req struct {
		Capacity      string   `json:"capacity"`
		AccessModes   []string `json:"access_modes"`
		ReclaimPolicy string   `json:"reclaim_policy"`
		StorageClass  string   `json:"storage_class"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()

	// 获取现有 PV
	pv, err := client.Clientset.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "pv not found")
		return
	}

	// 更新字段
	if req.Capacity != "" {
		capacity, err := resource.ParseQuantity(req.Capacity)
		if err != nil {
			response.BadRequest(c, "invalid capacity: "+err.Error())
			return
		}
		pv.Spec.Capacity = corev1.ResourceList{
			corev1.ResourceStorage: capacity,
		}
	}

	if len(req.AccessModes) > 0 {
		accessModes := make([]corev1.PersistentVolumeAccessMode, 0)
		for _, mode := range req.AccessModes {
			accessModes = append(accessModes, corev1.PersistentVolumeAccessMode(mode))
		}
		pv.Spec.AccessModes = accessModes
	}

	if req.ReclaimPolicy != "" {
		pv.Spec.PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimPolicy(req.ReclaimPolicy)
	}

	if req.StorageClass != "" {
		pv.Spec.StorageClassName = req.StorageClass
	}

	result, err := client.Clientset.CoreV1().PersistentVolumes().Update(ctx, pv, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// UpdatePVC 更新 PersistentVolumeClaim
func (h *Handler) UpdatePVC(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	var req struct {
		Capacity     string   `json:"capacity"`
		AccessModes  []string `json:"access_modes"`
		StorageClass string   `json:"storage_class"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()

	// 获取现有 PVC
	pvc, err := client.Clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "pvc not found")
		return
	}

	// 更新字段
	if req.Capacity != "" {
		capacity, err := resource.ParseQuantity(req.Capacity)
		if err != nil {
			response.BadRequest(c, "invalid capacity: "+err.Error())
			return
		}
		pvc.Spec.Resources = corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: capacity,
			},
		}
	}

	if len(req.AccessModes) > 0 {
		accessModes := make([]corev1.PersistentVolumeAccessMode, 0)
		for _, mode := range req.AccessModes {
			accessModes = append(accessModes, corev1.PersistentVolumeAccessMode(mode))
		}
		pvc.Spec.AccessModes = accessModes
	}

	if req.StorageClass != "" {
		pvc.Spec.StorageClassName = &req.StorageClass
	}

	result, err := client.Clientset.CoreV1().PersistentVolumeClaims(namespace).Update(ctx, pvc, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// GetStorageClass 获取 StorageClass 详情
func (h *Handler) GetStorageClass(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	sc, err := client.Clientset.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "storageclass not found")
		return
	}

	response.Success(c, sc)
}

// CreateStorageClass 创建 StorageClass
func (h *Handler) CreateStorageClass(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		Name              string            `json:"name" binding:"required"`
		Provisioner       string            `json:"provisioner" binding:"required"`
		ReclaimPolicy     string            `json:"reclaim_policy"`
		VolumeBindingMode string            `json:"volume_binding_mode"`
		Parameters        map[string]string `json:"parameters"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if req.ReclaimPolicy == "" {
		req.ReclaimPolicy = "Delete"
	}

	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Name,
		},
		Provisioner:   req.Provisioner,
		ReclaimPolicy: func() *corev1.PersistentVolumeReclaimPolicy { p := corev1.PersistentVolumeReclaimPolicy(req.ReclaimPolicy); return &p }(),
		Parameters:    req.Parameters,
	}

	if req.VolumeBindingMode != "" {
		mode := storagev1.VolumeBindingMode(req.VolumeBindingMode)
		sc.VolumeBindingMode = &mode
	}

	ctx := context.Background()
	result, err := client.Clientset.StorageV1().StorageClasses().Create(ctx, sc, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

// UpdateStorageClass 更新 StorageClass
func (h *Handler) UpdateStorageClass(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	name := c.Param("name")

	var req struct {
		Provisioner       string            `json:"provisioner"`
		ReclaimPolicy     string            `json:"reclaim_policy"`
		VolumeBindingMode string            `json:"volume_binding_mode"`
		Parameters        map[string]string `json:"parameters"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()

	// 获取现有 StorageClass
	sc, err := client.Clientset.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "storageclass not found")
		return
	}

	// 更新字段
	if req.Provisioner != "" {
		sc.Provisioner = req.Provisioner
	}

	if req.ReclaimPolicy != "" {
		policy := corev1.PersistentVolumeReclaimPolicy(req.ReclaimPolicy)
		sc.ReclaimPolicy = &policy
	}

	if req.VolumeBindingMode != "" {
		mode := storagev1.VolumeBindingMode(req.VolumeBindingMode)
		sc.VolumeBindingMode = &mode
	}

	if req.Parameters != nil {
		sc.Parameters = req.Parameters
	}

	result, err := client.Clientset.StorageV1().StorageClasses().Update(ctx, sc, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// DeleteStorageClass 删除 StorageClass
func (h *Handler) DeleteStorageClass(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	name := c.Param("name")

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	err = client.Clientset.StorageV1().StorageClasses().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "storageclass deleted", nil)
}

// Helper function
func timeSince(t time.Time) string {
	duration := time.Since(t)
	if duration.Hours() > 24*365 {
		return strconv.Itoa(int(duration.Hours()/24/365)) + "y"
	}
	if duration.Hours() > 24*30 {
		return strconv.Itoa(int(duration.Hours()/24/30)) + "Mo"
	}
	if duration.Hours() > 24 {
		return strconv.Itoa(int(duration.Hours()/24)) + "d"
	}
	if duration.Hours() > 1 {
		return strconv.Itoa(int(duration.Hours())) + "h"
	}
	if duration.Minutes() > 1 {
		return strconv.Itoa(int(duration.Minutes())) + "m"
	}
	return strconv.Itoa(int(duration.Seconds())) + "s"
}
