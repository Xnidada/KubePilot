package workload

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// CreateDeploymentRequest 企业级Deployment创建请求
type CreateDeploymentRequest struct {
	// 基本信息
	Name        string            `json:"name" binding:"required"`
	Namespace   string            `json:"namespace" binding:"required"`
	Description string            `json:"description"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`

	// 副本配置
	Replicas    int32 `json:"replicas"`
	MaxSurge    *int  `json:"max_surge"`
	MaxUnavail  *int  `json:"max_unavailable"`

	// 容器配置
	Containers []ContainerConfig `json:"containers" binding:"required,min=1"`

	// 初始化容器
	InitContainers []ContainerConfig `json:"init_containers"`

	// 数据存储
	Volumes []VolumeConfig `json:"volumes"`

	// 调度配置
	Scheduling *SchedulingConfig `json:"scheduling"`

	// 网络配置
	Network *NetworkConfig `json:"network"`

	// 高级配置
	Advanced *AdvancedConfig `json:"advanced"`
}

// ContainerConfig 容器配置
type ContainerConfig struct {
	Name            string            `json:"name" binding:"required"`
	Image           string            `json:"image" binding:"required"`
	ImagePullPolicy string            `json:"image_pull_policy"`
	Command         []string          `json:"command"`
	Args            []string          `json:"args"`
	WorkingDir      string            `json:"working_dir"`

	// 资源配额
	Resources *ResourceConfig `json:"resources"`

	// 端口配置
	Ports []ContainerPort `json:"ports"`

	// 环境变量
	Env []EnvVar `json:"env"`

	// 挂载卷
	VolumeMounts []VolumeMount `json:"volume_mounts"`

	// 健康检查
	LivenessProbe  *ProbeConfig `json:"liveness_probe"`
	ReadinessProbe *ProbeConfig `json:"readiness_probe"`
	StartupProbe   *ProbeConfig `json:"startup_probe"`

	// 生命周期
	Lifecycle *LifecycleConfig `json:"lifecycle"`

	// 安全上下文
	SecurityContext *ContainerSecurityContext `json:"security_context"`
}

// ResourceConfig 资源配置
type ResourceConfig struct {
	CPURequest    string `json:"cpu_request"`
	CPULimit      string `json:"cpu_limit"`
	MemoryRequest string `json:"memory_request"`
	MemoryLimit   string `json:"memory_limit"`
	GPURequest    string `json:"gpu_request"`
	GPULimit      string `json:"gpu_limit"`
}

// ContainerPort 端口配置
type ContainerPort struct {
	Name          string `json:"name"`
	ContainerPort int32  `json:"container_port" binding:"required"`
	Protocol      string `json:"protocol"`
}

// EnvVar 环境变量
type EnvVar struct {
	Name      string `json:"name" binding:"required"`
	Value     string `json:"value"`
	ValueFrom *EnvVarSource `json:"value_from"`
}

// EnvVarSource 环境变量来源
type EnvVarSource struct {
	Type          string `json:"type"` // configmap, secret, field, resource
	ConfigMapName string `json:"configmap_name"`
	ConfigMapKey  string `json:"configmap_key"`
	SecretName    string `json:"secret_name"`
	SecretKey     string `json:"secret_key"`
	FieldPath     string `json:"field_path"`
}

// VolumeConfig 存储卷配置
type VolumeConfig struct {
	Name       string          `json:"name" binding:"required"`
	Type       string          `json:"type" binding:"required"` // emptydir, hostpath, nfs, configmap, secret, pvc
	EmptyDir   *EmptyDirConfig `json:"empty_dir"`
	HostPath   *HostPathConfig `json:"host_path"`
	NFS        *NFSConfig      `json:"nfs"`
	ConfigMap  string          `json:"configmap"`
	Secret     string          `json:"secret"`
	PVCName    string          `json:"pvc_name"`
}

// EmptyDirConfig EmptyDir配置
type EmptyDirConfig struct {
	Medium    string `json:"medium"` // memory or empty
	SizeLimit string `json:"size_limit"`
}

// HostPathConfig HostPath配置
type HostPathConfig struct {
	Path string `json:"path" binding:"required"`
	Type string `json:"type"` // DirectoryOrCreate, Directory, FileOrCreate, File, Socket, CharDevice, BlockDevice
}

// NFSConfig NFS配置
type NFSConfig struct {
	Server   string `json:"server" binding:"required"`
	Path     string `json:"path" binding:"required"`
	ReadOnly bool   `json:"read_only"`
}

// VolumeMount 挂载配置
type VolumeMount struct {
	Name      string `json:"name" binding:"required"`
	MountPath string `json:"mount_path" binding:"required"`
	SubPath   string `json:"sub_path"`
	ReadOnly  bool   `json:"read_only"`
}

// ProbeConfig 健康检查配置
type ProbeConfig struct {
	ProbeType        string        `json:"probe_type"` // http, tcp, exec, grpc
	HTTPGet          *HTTPGetProbe `json:"http_get"`
	TCPSocket        *TCPSocket    `json:"tcp_socket"`
	Exec             *ExecProbe    `json:"exec"`
	InitialDelaySecs int32         `json:"initial_delay_seconds"`
	PeriodSecs       int32         `json:"period_seconds"`
	TimeoutSecs      int32         `json:"timeout_seconds"`
	SuccessThres     int32         `json:"success_threshold"`
	FailureThres     int32         `json:"failure_threshold"`
}

// HTTPGetProbe HTTP探针
type HTTPGetProbe struct {
	Path   string            `json:"path"`
	Port   int32             `json:"port"`
	Scheme string            `json:"scheme"`
	Headers map[string]string `json:"headers"`
}

// TCPSocket TCP探针
type TCPSocket struct {
	Port int32 `json:"port"`
}

// ExecProbe 命令探针
type ExecProbe struct {
	Command []string `json:"command"`
}

// LifecycleConfig 生命周期配置
type LifecycleConfig struct {
	PostStart *LifecycleHandler `json:"post_start"`
	PreStop   *LifecycleHandler `json:"pre_stop"`
}

// LifecycleHandler 生命周期处理器
type LifecycleHandler struct {
	Type    string   `json:"type"` // exec, http
	Command []string `json:"command"`
	HTTPGet *HTTPGetProbe `json:"http_get"`
}

// ContainerSecurityContext 容器安全上下文
type ContainerSecurityContext struct {
	Privileged             *bool `json:"privileged"`
	ReadOnlyRootFilesystem *bool `json:"read_only_root_filesystem"`
	RunAsUser              *int64 `json:"run_as_user"`
	RunAsGroup             *int64 `json:"run_as_group"`
	RunAsNonRoot           *bool `json:"run_as_non_root"`
	AllowPrivilegeEscalation *bool `json:"allow_privilege_escalation"`
}

// SchedulingConfig 调度配置
type SchedulingConfig struct {
	// 节点选择器
	NodeSelector map[string]string `json:"node_selector"`

	// 节点亲和性
	NodeAffinity []NodeAffinityTerm `json:"node_affinity"`

	// Pod亲和性
	PodAffinity []PodAffinityTerm `json:"pod_affinity"`

	// Pod反亲和性
	PodAntiAffinity []PodAffinityTerm `json:"pod_anti_affinity"`

	// 污点容忍
	Tolerations []Toleration `json:"tolerations"`

	// 拓扑分布约束
	TopologySpreadConstraints []TopologySpreadConstraint `json:"topology_spread_constraints"`
}

// NodeAffinityTerm 节点亲和性条件
type NodeAffinityTerm struct {
	Weight     int32              `json:"weight"`
	Key        string             `json:"key" binding:"required"`
	Operator   string             `json:"operator" binding:"required"` // In, NotIn, Exists, DoesNotExist, Gt, Lt
	Values     []string           `json:"values"`
}

// PodAffinityTerm Pod亲和性条件
type PodAffinityTerm struct {
	Weight          int32             `json:"weight"`
	TopologyKey     string            `json:"topology_key" binding:"required"`
	LabelSelector   map[string]string `json:"label_selector"`
	Namespaces      []string          `json:"namespaces"`
}

// Toleration 污点容忍
type Toleration struct {
	Key      string `json:"key"`
	Operator string `json:"operator"` // Equal, Exists
	Value    string `json:"value"`
	Effect   string `json:"effect"` // NoSchedule, PreferNoSchedule, NoExecute
}

// TopologySpreadConstraint 拓扑分布约束
type TopologySpreadConstraint struct {
	MaxSkew           int32             `json:"max_skew"`
	TopologyKey       string            `json:"topology_key"`
	WhenUnsatisfiable string            `json:"when_unsatisfiable"` // DoNotSchedule, ScheduleAnyway
	LabelSelector     map[string]string `json:"label_selector"`
}

// NetworkConfig 网络配置
type NetworkConfig struct {
	// 创建Service
	CreateService bool           `json:"create_service"`
	ServiceConfig *ServiceConfig `json:"service_config"`

	// 创建Ingress
	CreateIngress bool           `json:"create_ingress"`
	IngressConfig *IngressConfig `json:"ingress_config"`
}

// ServiceConfig Service配置
type ServiceConfig struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"` // ClusterIP, NodePort, LoadBalancer
	Ports       []ServicePort     `json:"ports"`
	Annotations map[string]string `json:"annotations"`
}

// ServicePort Service端口
type ServicePort struct {
	Name       string `json:"name"`
	Port       int32  `json:"port" binding:"required"`
	TargetPort int32  `json:"target_port" binding:"required"`
	NodePort   int32  `json:"node_port"`
	Protocol   string `json:"protocol"`
}

// IngressConfig Ingress配置
type IngressConfig struct {
	Name        string            `json:"name"`
	ClassName   string            `json:"class_name"`
	Annotations map[string]string `json:"annotations"`
	Host        string            `json:"host" binding:"required"`
	Path        string            `json:"path"`
	PathType    string            `json:"path_type"`
	TLSSecret   string            `json:"tls_secret"`
}

// AdvancedConfig 高级配置
type AdvancedConfig struct {
	// 服务账号
	ServiceAccountName string `json:"service_account_name"`

	// Pod安全上下文
	PodSecurityContext *PodSecurityContext `json:"pod_security_context"`

	// 优雅终止时间
	TerminationGracePeriodSecs *int64 `json:"termination_grace_period_seconds"`

	// DNS策略
	DNSPolicy string `json:"dns_policy"` // ClusterFirst, Default, ClusterFirstWithHostNet, None

	// 主机网络
	HostNetwork bool `json:"host_network"`

	// 主机PID
	HostPID bool `json:"host_pid"`

	// 镜像拉取密钥
	ImagePullSecrets []string `json:"image_pull_secrets"`

	// 重启策略
	RestartPolicy string `json:"restart_policy"` // Always, OnFailure, Never
}

// PodSecurityContext Pod安全上下文
type PodSecurityContext struct {
	RunAsUser    *int64 `json:"run_as_user"`
	RunAsGroup   *int64 `json:"run_as_group"`
	RunAsNonRoot *bool  `json:"run_as_non_root"`
	FSGroup      *int64 `json:"fs_group"`
}

// CreateEnterpriseDeployment 企业级创建Deployment
func (h *Handler) CreateEnterpriseDeployment(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req CreateDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 设置默认值
	if req.Replicas == 0 {
		req.Replicas = 1
	}

	// 构建Labels - 确保app标签存在
	labels := map[string]string{
		"app": req.Name,
	}
	// 用户自定义标签会覆盖默认值
	for k, v := range req.Labels {
		labels[k] = v
	}

	// 构建Selector - 必须与template labels匹配
	selector := map[string]string{
		"app": labels["app"],
	}

	// 构建容器
	containers := buildContainers(req.Containers)

	// 构建初始化容器
	initContainers := buildContainers(req.InitContainers)

	// 构建卷
	volumes := buildVolumes(req.Volumes)

	// 构建Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Namespace:   req.Namespace,
			Labels:      labels,
			Annotations: req.Annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &req.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: req.Annotations,
				},
				Spec: corev1.PodSpec{
					Containers:     containers,
					InitContainers: initContainers,
					Volumes:        volumes,
				},
			},
		},
	}

	// 设置滚动更新策略
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

	// 设置调度配置
	if req.Scheduling != nil {
		deployment.Spec.Template.Spec.NodeSelector = req.Scheduling.NodeSelector

		if len(req.Scheduling.Tolerations) > 0 {
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

	// 设置高级配置
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
		if req.Advanced.HostPID {
			deployment.Spec.Template.Spec.HostPID = true
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

	ctx := context.Background()
	result, err := client.Clientset.AppsV1().Deployments(req.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	// 创建Service
	if req.Network != nil && req.Network.CreateService && req.Network.ServiceConfig != nil {
		service := buildService(req.Name, req.Namespace, labels, selector, req.Network.ServiceConfig)
		_, err := client.Clientset.CoreV1().Services(req.Namespace).Create(ctx, service, metav1.CreateOptions{})
		if err != nil {
			// Service创建失败不影响Deployment
			response.SuccessWithMessage(c, "deployment created, but service creation failed: "+err.Error(), result)
			return
		}
	}

	response.Created(c, result)
}

// buildContainers 构建容器列表
func buildContainers(configs []ContainerConfig) []corev1.Container {
	containers := make([]corev1.Container, 0, len(configs))

	for _, cfg := range configs {
		container := corev1.Container{
			Name:       cfg.Name,
			Image:      cfg.Image,
			Command:    cfg.Command,
			Args:       cfg.Args,
			WorkingDir: cfg.WorkingDir,
		}

		// 镜像拉取策略
		if cfg.ImagePullPolicy != "" {
			container.ImagePullPolicy = corev1.PullPolicy(cfg.ImagePullPolicy)
		}

		// 资源配额
		if cfg.Resources != nil {
			container.Resources = buildResourceRequirements(cfg.Resources)
		}

		// 端口
		if len(cfg.Ports) > 0 {
			ports := make([]corev1.ContainerPort, 0)
			for _, p := range cfg.Ports {
				protocol := corev1.ProtocolTCP
				if p.Protocol == "UDP" {
					protocol = corev1.ProtocolUDP
				} else if p.Protocol == "SCTP" {
					protocol = corev1.ProtocolSCTP
				}
				ports = append(ports, corev1.ContainerPort{
					Name:          p.Name,
					ContainerPort: p.ContainerPort,
					Protocol:      protocol,
				})
			}
			container.Ports = ports
		}

		// 环境变量
		if len(cfg.Env) > 0 {
			envVars := buildEnvVars(cfg.Env)
			container.Env = envVars
		}

		// 挂载卷
		if len(cfg.VolumeMounts) > 0 {
			mounts := make([]corev1.VolumeMount, 0)
			for _, m := range cfg.VolumeMounts {
				mounts = append(mounts, corev1.VolumeMount{
					Name:      m.Name,
					MountPath: m.MountPath,
					SubPath:   m.SubPath,
					ReadOnly:  m.ReadOnly,
				})
			}
			container.VolumeMounts = mounts
		}

		// 健康检查
		if cfg.LivenessProbe != nil {
			container.LivenessProbe = buildProbe(cfg.LivenessProbe)
		}
		if cfg.ReadinessProbe != nil {
			container.ReadinessProbe = buildProbe(cfg.ReadinessProbe)
		}
		if cfg.StartupProbe != nil {
			container.StartupProbe = buildProbe(cfg.StartupProbe)
		}

		// 生命周期
		if cfg.Lifecycle != nil {
			container.Lifecycle = buildLifecycle(cfg.Lifecycle)
		}

		// 安全上下文
		if cfg.SecurityContext != nil {
			container.SecurityContext = &corev1.SecurityContext{
				Privileged:               cfg.SecurityContext.Privileged,
				ReadOnlyRootFilesystem:   cfg.SecurityContext.ReadOnlyRootFilesystem,
				RunAsUser:                cfg.SecurityContext.RunAsUser,
				RunAsGroup:               cfg.SecurityContext.RunAsGroup,
				RunAsNonRoot:             cfg.SecurityContext.RunAsNonRoot,
				AllowPrivilegeEscalation: cfg.SecurityContext.AllowPrivilegeEscalation,
			}
		}

		containers = append(containers, container)
	}

	return containers
}

// buildResourceRequirements 构建资源需求
func buildResourceRequirements(cfg *ResourceConfig) corev1.ResourceRequirements {
	requirements := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}

	if cfg.CPURequest != "" {
		if q, err := resource.ParseQuantity(cfg.CPURequest); err == nil {
			requirements.Requests[corev1.ResourceCPU] = q
		}
	}
	if cfg.CPULimit != "" {
		if q, err := resource.ParseQuantity(cfg.CPULimit); err == nil {
			requirements.Limits[corev1.ResourceCPU] = q
		}
	}
	if cfg.MemoryRequest != "" {
		if q, err := resource.ParseQuantity(cfg.MemoryRequest); err == nil {
			requirements.Requests[corev1.ResourceMemory] = q
		}
	}
	if cfg.MemoryLimit != "" {
		if q, err := resource.ParseQuantity(cfg.MemoryLimit); err == nil {
			requirements.Limits[corev1.ResourceMemory] = q
		}
	}
	if cfg.GPURequest != "" {
		if q, err := resource.ParseQuantity(cfg.GPURequest); err == nil {
			requirements.Requests["nvidia.com/gpu"] = q
		}
	}
	if cfg.GPULimit != "" {
		if q, err := resource.ParseQuantity(cfg.GPULimit); err == nil {
			requirements.Limits["nvidia.com/gpu"] = q
		}
	}

	return requirements
}

// buildEnvVars 构建环境变量
func buildEnvVars(envs []EnvVar) []corev1.EnvVar {
	result := make([]corev1.EnvVar, 0, len(envs))

	for _, e := range envs {
		envVar := corev1.EnvVar{
			Name:  e.Name,
			Value: e.Value,
		}

		if e.ValueFrom != nil {
			switch e.ValueFrom.Type {
			case "configmap":
				envVar.ValueFrom = &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: e.ValueFrom.ConfigMapName,
						},
						Key: e.ValueFrom.ConfigMapKey,
					},
				}
			case "secret":
				envVar.ValueFrom = &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: e.ValueFrom.SecretName,
						},
						Key: e.ValueFrom.SecretKey,
					},
				}
			case "field":
				envVar.ValueFrom = &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: e.ValueFrom.FieldPath,
					},
				}
			}
			envVar.Value = ""
		}

		result = append(result, envVar)
	}

	return result
}

// buildVolumes 构建卷列表
func buildVolumes(configs []VolumeConfig) []corev1.Volume {
	volumes := make([]corev1.Volume, 0, len(configs))

	for _, cfg := range configs {
		volume := corev1.Volume{
			Name: cfg.Name,
		}

		switch cfg.Type {
		case "emptydir":
			ed := &corev1.EmptyDirVolumeSource{}
			if cfg.EmptyDir != nil {
				if cfg.EmptyDir.Medium == "memory" {
					ed.Medium = corev1.StorageMediumMemory
				}
				if cfg.EmptyDir.SizeLimit != "" {
					if q, err := resource.ParseQuantity(cfg.EmptyDir.SizeLimit); err == nil {
						ed.SizeLimit = &q
					}
				}
			}
			volume.VolumeSource = corev1.VolumeSource{EmptyDir: ed}

		case "hostpath":
			if cfg.HostPath != nil {
				hostPathType := corev1.HostPathDirectoryOrCreate
				switch cfg.HostPath.Type {
				case "Directory":
					hostPathType = corev1.HostPathDirectory
				case "FileOrCreate":
					hostPathType = corev1.HostPathFileOrCreate
				case "File":
					hostPathType = corev1.HostPathFile
				case "Socket":
					hostPathType = corev1.HostPathSocket
				case "CharDevice":
					hostPathType = corev1.HostPathCharDev
				case "BlockDevice":
					hostPathType = corev1.HostPathBlockDev
				}
				volume.VolumeSource = corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: cfg.HostPath.Path,
						Type: &hostPathType,
					},
				}
			}

		case "nfs":
			if cfg.NFS != nil {
				volume.VolumeSource = corev1.VolumeSource{
					NFS: &corev1.NFSVolumeSource{
						Server:   cfg.NFS.Server,
						Path:     cfg.NFS.Path,
						ReadOnly: cfg.NFS.ReadOnly,
					},
				}
			}

		case "configmap":
			volume.VolumeSource = corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cfg.ConfigMap,
					},
				},
			}

		case "secret":
			volume.VolumeSource = corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cfg.Secret,
				},
			}

		case "pvc":
			volume.VolumeSource = corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: cfg.PVCName,
				},
			}
		}

		volumes = append(volumes, volume)
	}

	return volumes
}

// buildProbe 构建探针
func buildProbe(cfg *ProbeConfig) *corev1.Probe {
	probe := &corev1.Probe{
		InitialDelaySeconds: cfg.InitialDelaySecs,
		PeriodSeconds:       cfg.PeriodSecs,
		TimeoutSeconds:      cfg.TimeoutSecs,
		SuccessThreshold:    cfg.SuccessThres,
		FailureThreshold:    cfg.FailureThres,
	}

	if probe.PeriodSeconds == 0 {
		probe.PeriodSeconds = 10
	}
	if probe.TimeoutSeconds == 0 {
		probe.TimeoutSeconds = 1
	}
	if probe.SuccessThreshold == 0 {
		probe.SuccessThreshold = 1
	}
	if probe.FailureThreshold == 0 {
		probe.FailureThreshold = 3
	}

	switch cfg.ProbeType {
	case "http":
		if cfg.HTTPGet != nil {
			scheme := corev1.URISchemeHTTP
			if cfg.HTTPGet.Scheme == "https" {
				scheme = corev1.URISchemeHTTPS
			}
			headers := make([]corev1.HTTPHeader, 0)
			for k, v := range cfg.HTTPGet.Headers {
				headers = append(headers, corev1.HTTPHeader{Name: k, Value: v})
			}
			probe.ProbeHandler = corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:        cfg.HTTPGet.Path,
					Port:        intstr.FromInt(int(cfg.HTTPGet.Port)),
					Scheme:      scheme,
					HTTPHeaders: headers,
				},
			}
		}
	case "tcp":
		if cfg.TCPSocket != nil {
			probe.ProbeHandler = corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(int(cfg.TCPSocket.Port)),
				},
			}
		}
	case "exec":
		if cfg.Exec != nil {
			probe.ProbeHandler = corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: cfg.Exec.Command,
				},
			}
		}
	}

	return probe
}

// buildLifecycle 构建生命周期
func buildLifecycle(cfg *LifecycleConfig) *corev1.Lifecycle {
	lifecycle := &corev1.Lifecycle{}

	if cfg.PostStart != nil {
		lifecycle.PostStart = buildLifecycleHandler(cfg.PostStart)
	}
	if cfg.PreStop != nil {
		lifecycle.PreStop = buildLifecycleHandler(cfg.PreStop)
	}

	return lifecycle
}

// buildLifecycleHandler 构建生命周期处理器
func buildLifecycleHandler(cfg *LifecycleHandler) *corev1.LifecycleHandler {
	handler := &corev1.LifecycleHandler{}

	switch cfg.Type {
	case "exec":
		if len(cfg.Command) > 0 {
			handler.Exec = &corev1.ExecAction{
				Command: cfg.Command,
			}
		}
	case "http":
		if cfg.HTTPGet != nil {
			scheme := corev1.URISchemeHTTP
			if cfg.HTTPGet.Scheme == "https" {
				scheme = corev1.URISchemeHTTPS
			}
			handler.HTTPGet = &corev1.HTTPGetAction{
				Path:   cfg.HTTPGet.Path,
				Port:   intstr.FromInt(int(cfg.HTTPGet.Port)),
				Scheme: scheme,
			}
		}
	}

	return handler
}

// buildService 构建Service
func buildService(name, namespace string, labels map[string]string, selector map[string]string, cfg *ServiceConfig) *corev1.Service {
	svcName := cfg.Name
	if svcName == "" {
		svcName = name
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        svcName,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: cfg.Annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceType(cfg.Type),
			Selector: selector,
			Ports:    buildServicePorts(cfg.Ports),
		},
	}

	return service
}

// buildServicePorts 构建Service端口
func buildServicePorts(configs []ServicePort) []corev1.ServicePort {
	ports := make([]corev1.ServicePort, 0, len(configs))

	for i, cfg := range configs {
		portName := cfg.Name
		if portName == "" {
			portName = "port-" + strconv.Itoa(i)
		}

		protocol := corev1.ProtocolTCP
		if cfg.Protocol == "UDP" {
			protocol = corev1.ProtocolUDP
		} else if cfg.Protocol == "SCTP" {
			protocol = corev1.ProtocolSCTP
		}

		svcPort := corev1.ServicePort{
			Name:       portName,
			Port:       cfg.Port,
			TargetPort: intstr.FromInt(int(cfg.TargetPort)),
			Protocol:   protocol,
		}

		if cfg.NodePort > 0 {
			svcPort.NodePort = cfg.NodePort
		}

		ports = append(ports, svcPort)
	}

	return ports
}
