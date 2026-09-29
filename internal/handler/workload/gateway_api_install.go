package workload

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	gatewayInstallerVersion   = "v1.9.1"
	gatewayAPIBundleVersion   = "v1.6.1"
	gatewayInstallerNamespace = "envoy-gateway-system"
	gatewayInstallerURL       = "https://github.com/envoyproxy/gateway/releases/download/v1.9.1/install.yaml"
	gatewayInstallerSHA256    = "72b3971364f172eb0b9636c7142cc84ff695467bc065897958bde85a3c06cfd5"
)

var gatewayCRDGVR = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}

//go:embed assets/envoy-gateway-v1.9.1-install.yaml.gz
var gatewayInstallerManifestGzip []byte

type gatewayAPIInstallPlan struct {
	Installable       bool   `json:"installable"`
	Reason            string `json:"reason,omitempty"`
	ClusterName       string `json:"cluster_name"`
	Environment       string `json:"environment"`
	KubernetesVersion string `json:"kubernetes_version"`
	Controller        string `json:"controller"`
	ControllerVersion string `json:"controller_version"`
	GatewayAPIVersion string `json:"gateway_api_version"`
	Namespace         string `json:"namespace"`
	ManifestURL       string `json:"manifest_url"`
	ManifestSHA256    string `json:"manifest_sha256"`
}

func (h *Handler) GetGatewayAPIInstallPlan(c *gin.Context) {
	_, cluster, client, ok := gatewayInstallerTarget(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	plan, err := buildGatewayAPIInstallPlan(ctx, cluster, client)
	if err != nil {
		gatewayInstallCheckError(c, err)
		return
	}
	response.Success(c, plan)
}

func (h *Handler) InstallGatewayAPI(c *gin.Context) {
	c.Set("audit_resource_name", "envoy-gateway/"+gatewayInstallerVersion)
	if h.kubectlExecutor == nil || h.db == nil {
		response.InternalError(c, "kubectl executor unavailable")
		return
	}
	clusterID, cluster, client, ok := gatewayInstallerTarget(c)
	if !ok {
		return
	}
	var request struct {
		ConfirmCluster string `json:"confirm_cluster" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.ConfirmCluster != cluster.Name {
		response.BadRequest(c, "请准确输入集群名称以确认安装")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	plan, err := buildGatewayAPIInstallPlan(ctx, cluster, client)
	if err != nil {
		gatewayInstallCheckError(c, err)
		return
	}
	if !plan.Installable {
		response.Error(c, http.StatusConflict, plan.Reason)
		return
	}
	job := model.GatewayInstallJob{ClusterID: clusterID, UserID: c.GetUint("user_id"), Status: "pending", Message: "等待安装工作进程"}
	if err := h.db.Create(&job).Error; err != nil {
		var previous model.GatewayInstallJob
		if h.db.Where("cluster_id = ?", clusterID).First(&previous).Error != nil || previous.Status != "failed" {
			response.Error(c, http.StatusConflict, "该集群已有安装任务；请先查看任务状态，不会重复安装")
			return
		}
		// The preflight above proves no CRD/namespace exists, so retrying an
		// earlier failure cannot duplicate a partially applied installation.
		claim := h.db.Model(&previous).Where("status = ?", "failed").Updates(map[string]any{
			"status": "pending", "message": "安全预检通过，重新排队", "lease_until": nil, "finished_at": nil, "user_id": c.GetUint("user_id"),
		})
		if claim.Error != nil || claim.RowsAffected != 1 {
			response.Error(c, http.StatusConflict, "安装任务状态已变化，请刷新后重试")
			return
		}
		job = previous
		job.Status = "pending"
		_ = h.db.Create(&model.GatewayInstallLog{JobID: job.ID, Level: "info", Message: "失败任务经重新预检后重新排队", CreatedAt: time.Now().UTC()}).Error
	}
	c.Set("audit_resource_name", fmt.Sprintf("gateway_install_job:%d", job.ID))
	go h.runGatewayInstallJob(job.ID)
	c.JSON(http.StatusAccepted, response.Response{Data: gin.H{"job_id": job.ID, "status": job.Status}, Message: "安装任务已创建"})
}

// runGatewayInstall is shared by the background worker and its unit tests.
func (h *Handler) runGatewayInstall(ctx context.Context, clusterID uint, client kubernetes.Interface, manifestPath string, emit func(string, string)) (string, string) {
	type applyResult struct {
		success bool
		stderr  string
		err     error
	}
	results := make(chan applyResult, 1)
	go func() {
		args := []string{"apply", "--server-side", "-f", manifestPath}
		if executor, ok := h.kubectlExecutor.(interface {
			ExecuteKubectlStream(context.Context, uint, []string, func(string, string)) (bool, error)
		}); ok {
			success, err := executor.ExecuteKubectlStream(ctx, clusterID, args, emit)
			results <- applyResult{success: success, err: err}
			return
		}
		success, output, stderr, err := h.kubectlExecutor.ExecuteKubectl(ctx, clusterID, args)
		if strings.TrimSpace(output) != "" {
			emit("info", "kubectl apply 输出:\n"+output)
		}
		if strings.TrimSpace(stderr) != "" {
			emit("error", stderr)
		}
		results <- applyResult{success: success, stderr: stderr, err: err}
	}()
	progress := time.NewTicker(15 * time.Second)
	defer progress.Stop()
	var result applyResult
	for {
		select {
		case result = <-results:
			goto applied
		case <-progress.C:
			emit("info", "kubectl apply 仍在执行，等待集群 API 返回结果")
		case <-ctx.Done():
			return "failed", "安装超时；可能已有部分资源，请先检查集群状态，勿重复提交"
		}
	}
applied:
	if result.err != nil || !result.success {
		return "failed", "安装清单应用失败；可能已有部分资源，请手动恢复: " + gatewayInstallFailure(result.err, result.stderr)
	}
	emit("success", "清单应用完成，开始检查 Envoy Gateway Deployment")
	ready, reason := waitGatewayController(ctx, client, func(message string) { emit("info", message) })
	if !ready {
		return "pending_controller", "安装清单已应用，但控制器尚未就绪：" + reason
	}
	emit("success", "Envoy Gateway Deployment 已就绪")
	return "ready", "Envoy Gateway 与 Gateway API CRD 已安装，控制器就绪"
}

func gatewayInstallLogText(message string) string {
	const maxLogBytes = 16 << 10
	message = strings.TrimSpace(message)
	if len(message) > maxLogBytes {
		message = strings.ToValidUTF8(message[:maxLogBytes], "") + "\n…输出已截断"
	}
	return message
}

func waitGatewayController(ctx context.Context, client kubernetes.Interface, emit func(string)) (bool, string) {
	waitCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		deployment, err := client.AppsV1().Deployments(gatewayInstallerNamespace).Get(waitCtx, "envoy-gateway", metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return false, "查询 Deployment 失败: " + err.Error()
		}
		if err == nil {
			if deployment.Status.AvailableReplicas > 0 && deployment.Status.ObservedGeneration >= deployment.Generation {
				return true, ""
			}
			var expected int32 = 1
			if deployment.Spec.Replicas != nil {
				expected = *deployment.Spec.Replicas
			}
			emit(fmt.Sprintf("等待控制器就绪：可用副本 %d，期望副本 %d", deployment.Status.AvailableReplicas, expected))
		} else {
			emit("等待 Envoy Gateway Deployment 创建")
		}
		select {
		case <-waitCtx.Done():
			return false, "75 秒内未就绪，请稍后刷新页面查看控制器状态"
		case <-ticker.C:
		}
	}
}

func gatewayInstallerTarget(c *gin.Context) (uint, model.Cluster, *k8s.ClusterClient, bool) {
	clusterID64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || clusterID64 == 0 {
		response.BadRequest(c, "invalid cluster id")
		return 0, model.Cluster{}, nil, false
	}
	clusterID := uint(clusterID64)
	var cluster model.Cluster
	if err := model.DB.Select("id", "name", "display_name", "environment").First(&cluster, clusterID).Error; err != nil {
		response.NotFound(c, "cluster not found")
		return 0, model.Cluster{}, nil, false
	}
	client, err := k8s.Manager.GetClient(clusterID)
	if err != nil {
		response.InternalError(c, "cluster unavailable: "+err.Error())
		return 0, model.Cluster{}, nil, false
	}
	return clusterID, cluster, client, true
}

func buildGatewayAPIInstallPlan(ctx context.Context, cluster model.Cluster, client *k8s.ClusterClient) (gatewayAPIInstallPlan, error) {
	plan := gatewayAPIInstallPlan{
		ClusterName: cluster.Name, Environment: cluster.Environment,
		Controller: "Envoy Gateway", ControllerVersion: gatewayInstallerVersion,
		GatewayAPIVersion: gatewayAPIBundleVersion, Namespace: gatewayInstallerNamespace,
		ManifestURL: gatewayInstallerURL, ManifestSHA256: gatewayInstallerSHA256,
	}
	version, err := client.Discovery.ServerVersion()
	if err != nil {
		return plan, fmt.Errorf("无法查询 Kubernetes 版本: %w", err)
	}
	plan.KubernetesVersion = version.GitVersion
	if !supportedGatewayKubernetesVersion(version.Major, version.Minor) {
		plan.Reason = "Envoy Gateway v1.9.1 仅支持 Kubernetes v1.33–v1.36；其他版本请使用手动安装流程"
		return plan, nil
	}
	clientMajor, clientMinor, err := gatewayKubectlVersion(ctx)
	if err != nil {
		plan.Reason = "应用容器缺少可用的 kubectl；请检查部署后再安装"
		return plan, nil
	}
	if !gatewayKubectlCompatible(version.Major, version.Minor, clientMajor, clientMinor) {
		plan.Reason = "应用容器的 kubectl 与目标集群版本相差超过 1 个次版本；请更新 kubectl 后再安装"
		return plan, nil
	}
	if _, err := loadPinnedGatewayManifest(gatewayInstallerSHA256); err != nil {
		return plan, fmt.Errorf("内置安装清单校验失败: %w", err)
	}
	dynamicClient, err := dynamic.NewForConfig(client.Config)
	if err != nil {
		return plan, err
	}
	reason, err := gatewayInstallPreflight(ctx, client.Clientset, dynamicClient)
	if err != nil {
		return plan, err
	}
	plan.Installable = reason == ""
	plan.Reason = reason
	return plan, nil
}

func supportedGatewayKubernetesVersion(major, minor string) bool {
	m, err := strconv.Atoi(major)
	if err != nil || m != 1 {
		return false
	}
	n, err := strconv.Atoi(strings.TrimRight(minor, "+"))
	return err == nil && n >= 33 && n <= 36
}

func gatewayKubectlVersion(ctx context.Context) (string, string, error) {
	output, err := exec.CommandContext(ctx, "kubectl", "version", "--client", "--output=json").Output()
	if err != nil {
		return "", "", err
	}
	var version struct {
		ClientVersion struct {
			Major string `json:"major"`
			Minor string `json:"minor"`
		} `json:"clientVersion"`
	}
	if err := json.Unmarshal(output, &version); err != nil {
		return "", "", err
	}
	return version.ClientVersion.Major, version.ClientVersion.Minor, nil
}

func gatewayKubectlCompatible(serverMajor, serverMinor, clientMajor, clientMinor string) bool {
	if serverMajor != clientMajor || serverMajor != "1" {
		return false
	}
	server, serverErr := strconv.Atoi(strings.TrimRight(serverMinor, "+"))
	client, clientErr := strconv.Atoi(strings.TrimRight(clientMinor, "+"))
	return serverErr == nil && clientErr == nil && server >= client-1 && server <= client+1
}

func gatewayInstallPreflight(ctx context.Context, client kubernetes.Interface, dynamicClient dynamic.Interface) (string, error) {
	continuation := ""
	for {
		crds, err := dynamicClient.Resource(gatewayCRDGVR).List(ctx, metav1.ListOptions{Limit: 500, Continue: continuation})
		if err != nil {
			return "", fmt.Errorf("无法检查现有 CRD: %w", err)
		}
		for _, crd := range crds.Items {
			if strings.HasSuffix(crd.GetName(), ".gateway.networking.k8s.io") || strings.HasSuffix(crd.GetName(), ".gateway.envoyproxy.io") {
				return "已检测到 Gateway API 或 Envoy Gateway CRD；为避免覆盖现有/托管 CRD，请走手动安装或恢复流程", nil
			}
		}
		continuation = crds.GetContinue()
		if continuation == "" {
			break
		}
	}
	_, err := client.CoreV1().Namespaces().Get(ctx, gatewayInstallerNamespace, metav1.GetOptions{})
	if err == nil {
		return "envoy-gateway-system 命名空间已存在；可能是已有控制器或部分安装，不会自动覆盖", nil
	}
	if !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("无法检查安装命名空间: %w", err)
	}
	return "", nil
}

func loadPinnedGatewayManifest(expectedSHA256 string) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(gatewayInstallerManifestGzip))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	const maxManifestSize = 8 << 20
	manifest, err := io.ReadAll(io.LimitReader(reader, maxManifestSize+1))
	if err != nil {
		return nil, err
	}
	if len(manifest) > maxManifestSize {
		return nil, fmt.Errorf("安装清单超过 8 MiB")
	}
	actual := sha256.Sum256(manifest)
	if hex.EncodeToString(actual[:]) != expectedSHA256 {
		return nil, fmt.Errorf("SHA-256 不匹配，拒绝执行")
	}
	return manifest, nil
}

func gatewayInstallCheckError(c *gin.Context, err error) {
	if apierrors.IsForbidden(err) {
		response.Forbidden(c, "目标集群 kubeconfig 无权检查或安装集群级资源: "+err.Error())
		return
	}
	response.InternalError(c, err.Error())
}

func gatewayInstallFailure(err error, stderr string) string {
	message := strings.TrimSpace(stderr)
	if message == "" && err != nil {
		message = err.Error()
	}
	if message == "" {
		message = "kubectl apply failed without diagnostics"
	}
	if len(message) > 1200 {
		message = message[:1200] + "…"
	}
	return message
}
