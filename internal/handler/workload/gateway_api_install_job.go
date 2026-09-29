package workload

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const gatewayJobLease = 180 * time.Second // execution context is limited to 165 seconds

func (h *Handler) GetGatewayInstallJob(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || clusterID == 0 {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	var job model.GatewayInstallJob
	err = h.db.Where("cluster_id = ?", clusterID).First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.Success(c, gin.H{"job": nil, "logs": []model.GatewayInstallLog{}})
		return
	}
	if err != nil {
		response.InternalError(c, "failed to load install job")
		return
	}
	if job.Status == "pending_controller" {
		checkCtx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		if client, err := k8s.Manager.GetClient(uint(clusterID)); err == nil && gatewayEnvoyControllerStatus(checkCtx, client.Clientset) == "ready" {
			now := time.Now()
			updated := h.db.Model(&job).Where("status = ?", "pending_controller").Updates(map[string]any{"status": "ready", "message": "Envoy Gateway 控制器已就绪", "finished_at": now})
			if updated.Error == nil && updated.RowsAffected == 1 {
				job.Status, job.Message, job.FinishedAt = "ready", "Envoy Gateway 控制器已就绪", &now
				_ = h.db.Create(&model.GatewayInstallLog{JobID: job.ID, Level: "success", Message: job.Message, CreatedAt: now}).Error
			}
		}
	}
	var logs []model.GatewayInstallLog
	if err := h.db.Where("job_id = ?", job.ID).Order("id DESC").Limit(250).Find(&logs).Error; err != nil {
		response.InternalError(c, "failed to load install logs")
		return
	}
	for left, right := 0, len(logs)-1; left < right; left, right = left+1, right-1 {
		logs[left], logs[right] = logs[right], logs[left]
	}
	response.Success(c, gin.H{"job": job, "logs": logs})
}

// StartGatewayInstallWorker recovers pending jobs and expired leases after a
// replica restart. The database claim prevents two replicas applying at once.
func (h *Handler) StartGatewayInstallWorker() {
	if h.db == nil || h.kubectlExecutor == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			var jobs []model.GatewayInstallJob
			_ = h.db.Where("status = ? OR (status = ? AND lease_until < ?)", "pending", "running", time.Now()).Limit(20).Find(&jobs).Error
			for _, job := range jobs {
				go h.runGatewayInstallJob(job.ID)
			}
			<-ticker.C
		}
	}()
}

func (h *Handler) runGatewayInstallJob(jobID uint) {
	var job model.GatewayInstallJob
	if err := h.db.First(&job, jobID).Error; err != nil {
		return
	}
	resuming := job.Status == "running"
	now := time.Now()
	lease := now.Add(gatewayJobLease)
	claim := h.db.Model(&model.GatewayInstallJob{}).Where("id = ? AND (status = ? OR (status = ? AND lease_until < ?))", jobID, "pending", "running", now).
		Updates(map[string]any{"status": "running", "lease_until": lease, "started_at": now})
	if claim.Error != nil || claim.RowsAffected != 1 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 165*time.Second)
	defer cancel()
	var logMu sync.Mutex
	logCount := 0
	emit := func(level, message string) {
		logMu.Lock()
		defer logMu.Unlock()
		if logCount >= 250 {
			return
		}
		logCount++
		if logCount == 250 {
			message = "日志行数已达 250 行上限，后续输出已截断"
			level = "warning"
		}
		_ = h.db.Create(&model.GatewayInstallLog{JobID: jobID, Level: level, Message: gatewayInstallLogText(message), CreatedAt: time.Now().UTC()}).Error
	}
	finish := func(status, message string) {
		level := "success"
		if status == "failed" {
			level = "error"
		} else if status == "pending_controller" {
			level = "warning"
		}
		logMu.Lock()
		if logCount >= 250 {
			logCount = 249
		}
		logMu.Unlock()
		emit(level, message)
		updates := map[string]any{"status": status, "message": message, "lease_until": nil}
		if status != "pending_controller" {
			updates["finished_at"] = time.Now()
		}
		_ = h.db.Model(&model.GatewayInstallJob{}).Where("id = ? AND status = ?", jobID, "running").Updates(updates).Error
	}
	client, err := k8s.Manager.GetClient(job.ClusterID)
	if err != nil {
		finish("failed", "集群不可用: "+err.Error())
		return
	}
	blocked, err := model.ProductionWriteBlocked(h.db, job.ClusterID)
	if err != nil || blocked {
		finish("failed", "生产变更策略已开启或检查失败；安装任务未执行，请使用独立变更预案")
		return
	}
	if !resuming {
		var cluster model.Cluster
		if err := h.db.First(&cluster, job.ClusterID).Error; err != nil {
			finish("failed", "集群记录不存在")
			return
		}
		plan, err := buildGatewayAPIInstallPlan(ctx, cluster, client)
		if err != nil || !plan.Installable {
			if err != nil {
				finish("failed", "安装预检失败: "+err.Error())
			} else {
				finish("failed", "安装预检失败: "+plan.Reason)
			}
			return
		}
	}
	namespace, err := client.Clientset.CoreV1().Namespaces().Get(ctx, gatewayInstallerNamespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		// Atomic namespace claim; any competing installer makes this fail closed.
		_, err = client.Clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: gatewayInstallerNamespace, Annotations: map[string]string{
				"kubepilot.io/gateway-api-installer": "envoy-gateway-" + gatewayInstallerVersion,
			},
		}}, metav1.CreateOptions{})
	} else if err == nil && (!resuming || namespace.Annotations["kubepilot.io/gateway-api-installer"] != "envoy-gateway-"+gatewayInstallerVersion) {
		finish("failed", "安装命名空间已存在且不属于本次任务，拒绝覆盖")
		return
	}
	if err != nil {
		finish("failed", "创建安装命名空间失败: "+err.Error())
		return
	}
	if resuming {
		emit("warning", "工作进程恢复，重新应用相同的固定清单（server-side apply 可重复执行）")
	} else {
		emit("success", "预检通过；已创建 envoy-gateway-system 命名空间")
	}
	manifest, err := loadPinnedGatewayManifest(gatewayInstallerSHA256)
	if err != nil {
		finish("failed", "内置安装清单校验失败: "+err.Error())
		return
	}
	file, err := os.CreateTemp("", "kubepilot-envoy-gateway-*.yaml")
	if err != nil {
		finish("failed", "创建临时安装清单失败")
		return
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(manifest); err != nil {
		file.Close()
		finish("failed", "写入临时安装清单失败")
		return
	}
	if err := file.Close(); err != nil {
		finish("failed", "关闭临时安装清单失败")
		return
	}
	emit("info", "开始应用 Envoy Gateway "+gatewayInstallerVersion+" 与 Gateway API "+gatewayAPIBundleVersion+" 清单")
	blocked, err = model.ProductionWriteBlocked(h.db, job.ClusterID)
	if err != nil || blocked {
		finish("failed", "生产变更策略已开启或检查失败；清单尚未应用，请人工检查命名空间后恢复")
		return
	}
	status, message := h.runGatewayInstall(ctx, job.ClusterID, client.Clientset, file.Name(), emit)
	finish(status, message)
}
