package workload

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ==================== StatefulSet ====================

// ListStatefulSets 获取StatefulSet列表
func (h *Handler) ListStatefulSets(c *gin.Context) {
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
	var statefulSets *appsv1.StatefulSetList
	if namespace == "" {
		statefulSets, err = client.Clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	} else {
		statefulSets, err = client.Clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type StatefulSetInfo struct {
		Name      string   `json:"name"`
		Namespace string   `json:"namespace"`
		Status    string   `json:"status"`
		Ready     string   `json:"ready"`
		Replicas  int32    `json:"replicas"`
		Age       string   `json:"age"`
		Images    []string `json:"images"`
	}

	result := make([]StatefulSetInfo, 0, len(statefulSets.Items))
	for _, sts := range statefulSets.Items {
		images := make([]string, 0)
		for _, c := range sts.Spec.Template.Spec.Containers {
			images = append(images, c.Image)
		}

		status := "Active"
		if sts.DeletionTimestamp != nil {
			status = "Terminating"
		}

		var replicas int32
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}

		result = append(result, StatefulSetInfo{
			Name:      sts.Name,
			Namespace: sts.Namespace,
			Status:    status,
			Ready:     fmt.Sprintf("%d/%d", sts.Status.ReadyReplicas, replicas),
			Replicas:  replicas,
			Age:       timeSince(sts.CreationTimestamp.Time),
			Images:    images,
		})
	}

	response.Success(c, result)
}

// DeleteStatefulSet 删除StatefulSet
func (h *Handler) DeleteStatefulSet(c *gin.Context) {
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
	err = client.Clientset.AppsV1().StatefulSets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "statefulset deleted", nil)
}

// ==================== DaemonSet ====================

// ListDaemonSets 获取DaemonSet列表
func (h *Handler) ListDaemonSets(c *gin.Context) {
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
	var daemonSets *appsv1.DaemonSetList
	if namespace == "" {
		daemonSets, err = client.Clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	} else {
		daemonSets, err = client.Clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type DaemonSetInfo struct {
		Name      string   `json:"name"`
		Namespace string   `json:"namespace"`
		Status    string   `json:"status"`
		Desired   int32    `json:"desired"`
		Current   int32    `json:"current"`
		Ready     int32    `json:"ready"`
		Age       string   `json:"age"`
		Images    []string `json:"images"`
	}

	result := make([]DaemonSetInfo, 0, len(daemonSets.Items))
	for _, ds := range daemonSets.Items {
		images := make([]string, 0)
		for _, c := range ds.Spec.Template.Spec.Containers {
			images = append(images, c.Image)
		}

		status := "Active"
		if ds.DeletionTimestamp != nil {
			status = "Terminating"
		}

		result = append(result, DaemonSetInfo{
			Name:      ds.Name,
			Namespace: ds.Namespace,
			Status:    status,
			Desired:   ds.Status.DesiredNumberScheduled,
			Current:   ds.Status.CurrentNumberScheduled,
			Ready:     ds.Status.NumberReady,
			Age:       timeSince(ds.CreationTimestamp.Time),
			Images:    images,
		})
	}

	response.Success(c, result)
}

// DeleteDaemonSet 删除DaemonSet
func (h *Handler) DeleteDaemonSet(c *gin.Context) {
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
	err = client.Clientset.AppsV1().DaemonSets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "daemonset deleted", nil)
}

// ==================== Job ====================

// ListJobs 获取Job列表
func (h *Handler) ListJobs(c *gin.Context) {
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
	var jobs *batchv1.JobList
	if namespace == "" {
		jobs, err = client.Clientset.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
	} else {
		jobs, err = client.Clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type JobInfo struct {
		Name        string   `json:"name"`
		Namespace   string   `json:"namespace"`
		Status      string   `json:"status"`
		Completions int32    `json:"completions"`
		Succeeded   int32    `json:"succeeded"`
		Age         string   `json:"age"`
		Images      []string `json:"images"`
	}

	result := make([]JobInfo, 0, len(jobs.Items))
	for _, job := range jobs.Items {
		images := make([]string, 0)
		for _, c := range job.Spec.Template.Spec.Containers {
			images = append(images, c.Image)
		}

		status := "Active"
		if job.DeletionTimestamp != nil {
			status = "Terminating"
		} else if job.Status.Succeeded > 0 && job.Spec.Completions != nil && job.Status.Succeeded >= *job.Spec.Completions {
			status = "Complete"
		} else if job.Status.Failed > 0 {
			status = "Failed"
		}

		var completions int32
		if job.Spec.Completions != nil {
			completions = *job.Spec.Completions
		}

		result = append(result, JobInfo{
			Name:        job.Name,
			Namespace:   job.Namespace,
			Status:      status,
			Completions: completions,
			Succeeded:   job.Status.Succeeded,
			Age:         timeSince(job.CreationTimestamp.Time),
			Images:      images,
		})
	}

	response.Success(c, result)
}

// DeleteJob 删除Job
func (h *Handler) DeleteJob(c *gin.Context) {
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
	err = client.Clientset.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "job deleted", nil)
}

// ==================== CronJob ====================

// ListCronJobs 获取CronJob列表
func (h *Handler) ListCronJobs(c *gin.Context) {
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
	var cronJobs *batchv1.CronJobList
	if namespace == "" {
		cronJobs, err = client.Clientset.BatchV1().CronJobs("").List(ctx, metav1.ListOptions{})
	} else {
		cronJobs, err = client.Clientset.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type CronJobInfo struct {
		Name        string   `json:"name"`
		Namespace   string   `json:"namespace"`
		Schedule    string   `json:"schedule"`
		Suspend     bool     `json:"suspend"`
		Active      int      `json:"active"`
		LastSchedule string  `json:"last_schedule"`
		Age         string   `json:"age"`
		Images      []string `json:"images"`
	}

	result := make([]CronJobInfo, 0, len(cronJobs.Items))
	for _, cj := range cronJobs.Items {
		images := make([]string, 0)
		for _, c := range cj.Spec.JobTemplate.Spec.Template.Spec.Containers {
			images = append(images, c.Image)
		}

		lastSchedule := ""
		if cj.Status.LastScheduleTime != nil {
			lastSchedule = timeSince(cj.Status.LastScheduleTime.Time)
		}

		suspend := false
		if cj.Spec.Suspend != nil {
			suspend = *cj.Spec.Suspend
		}

		result = append(result, CronJobInfo{
			Name:         cj.Name,
			Namespace:    cj.Namespace,
			Schedule:     cj.Spec.Schedule,
			Suspend:      suspend,
			Active:       len(cj.Status.Active),
			LastSchedule: lastSchedule,
			Age:          timeSince(cj.CreationTimestamp.Time),
			Images:       images,
		})
	}

	response.Success(c, result)
}

// DeleteCronJob 删除CronJob
func (h *Handler) DeleteCronJob(c *gin.Context) {
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
	err = client.Clientset.BatchV1().CronJobs(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "cronjob deleted", nil)
}

// ==================== ReplicaSet ====================

// ListReplicaSets 获取ReplicaSet列表
func (h *Handler) ListReplicaSets(c *gin.Context) {
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
	var replicaSets *appsv1.ReplicaSetList
	if namespace == "" {
		replicaSets, err = client.Clientset.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})
	} else {
		replicaSets, err = client.Clientset.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type ReplicaSetInfo struct {
		Name      string   `json:"name"`
		Namespace string   `json:"namespace"`
		Status    string   `json:"status"`
		Ready     string   `json:"ready"`
		Replicas  int32    `json:"replicas"`
		Age       string   `json:"age"`
		Images    []string `json:"images"`
		Owner     string   `json:"owner"`
	}

	result := make([]ReplicaSetInfo, 0, len(replicaSets.Items))
	for _, rs := range replicaSets.Items {
		images := make([]string, 0)
		for _, c := range rs.Spec.Template.Spec.Containers {
			images = append(images, c.Image)
		}

		status := "Active"
		if rs.DeletionTimestamp != nil {
			status = "Terminating"
		}

		var replicas int32
		if rs.Spec.Replicas != nil {
			replicas = *rs.Spec.Replicas
		}

		owner := ""
		for _, ref := range rs.OwnerReferences {
			owner = fmt.Sprintf("%s/%s", ref.Kind, ref.Name)
		}

		result = append(result, ReplicaSetInfo{
			Name:      rs.Name,
			Namespace: rs.Namespace,
			Status:    status,
			Ready:     fmt.Sprintf("%d/%d", rs.Status.ReadyReplicas, replicas),
			Replicas:  replicas,
			Age:       timeSince(rs.CreationTimestamp.Time),
			Images:    images,
			Owner:     owner,
		})
	}

	response.Success(c, result)
}

// DeleteReplicaSet 删除ReplicaSet
func (h *Handler) DeleteReplicaSet(c *gin.Context) {
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
	err = client.Clientset.AppsV1().ReplicaSets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "replicaset deleted", nil)
}

// GetReplicaSet 获取ReplicaSet详情
func (h *Handler) GetReplicaSet(c *gin.Context) {
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
	rs, err := client.Clientset.AppsV1().ReplicaSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "replicaset not found")
		return
	}

	// 获取关联的Pods
	pods, _ := client.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(rs.Spec.Selector),
	})

	podList := make([]map[string]interface{}, 0)
	if pods != nil {
		for _, pod := range pods.Items {
			podList = append(podList, map[string]interface{}{
				"name":   pod.Name,
				"status": string(pod.Status.Phase),
				"ip":     pod.Status.PodIP,
				"node":   pod.Spec.NodeName,
			})
		}
	}

	result := map[string]interface{}{
		"name":       rs.Name,
		"namespace":  rs.Namespace,
		"labels":     rs.Labels,
		"selector":   rs.Spec.Selector.MatchLabels,
		"replicas":   rs.Spec.Replicas,
		"ready":      rs.Status.ReadyReplicas,
		"available":  rs.Status.AvailableReplicas,
		"pods":       podList,
		"created_at": rs.CreationTimestamp.Time,
	}

	response.Success(c, result)
}

// ScaleReplicaSet 调整ReplicaSet副本数
func (h *Handler) ScaleReplicaSet(c *gin.Context) {
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
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	rs, err := client.Clientset.AppsV1().ReplicaSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "replicaset not found")
		return
	}

	rs.Spec.Replicas = &req.Replicas
	_, err = client.Clientset.AppsV1().ReplicaSets(namespace).Update(ctx, rs, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.SuccessWithMessage(c, "replicaset scaled", nil)
}

// ==================== StatefulSet CRUD ====================

// GetStatefulSet 获取StatefulSet详情
func (h *Handler) GetStatefulSet(c *gin.Context) {
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
	sts, err := client.Clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "statefulset not found")
		return
	}

	response.Success(c, sts)
}

// CreateStatefulSet 创建StatefulSet
func (h *Handler) CreateStatefulSet(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		Namespace string `json:"namespace" binding:"required"`
		Name      string `json:"name" binding:"required"`
		Replicas  int32  `json:"replicas"`
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
		ports = append(ports, corev1.ContainerPort{ContainerPort: p.ContainerPort})
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &req.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": req.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": req.Name},
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
	result, err := client.Clientset.AppsV1().StatefulSets(req.Namespace).Create(ctx, sts, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

// UpdateStatefulSet 更新StatefulSet
func (h *Handler) UpdateStatefulSet(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	var req struct {
		Replicas *int32 `json:"replicas"`
		Image    string `json:"image"`
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
	sts, err := client.Clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "statefulset not found")
		return
	}

	if req.Replicas != nil {
		sts.Spec.Replicas = req.Replicas
	}
	if req.Image != "" && len(sts.Spec.Template.Spec.Containers) > 0 {
		sts.Spec.Template.Spec.Containers[0].Image = req.Image
	}

	result, err := client.Clientset.AppsV1().StatefulSets(namespace).Update(ctx, sts, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// ==================== DaemonSet CRUD ====================

// GetDaemonSet 获取DaemonSet详情
func (h *Handler) GetDaemonSet(c *gin.Context) {
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
	ds, err := client.Clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "daemonset not found")
		return
	}

	response.Success(c, ds)
}

// CreateDaemonSet 创建DaemonSet
func (h *Handler) CreateDaemonSet(c *gin.Context) {
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
		ports = append(ports, corev1.ContainerPort{ContainerPort: p.ContainerPort})
	}

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": req.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": req.Name},
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
	result, err := client.Clientset.AppsV1().DaemonSets(req.Namespace).Create(ctx, ds, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

// UpdateDaemonSet 更新DaemonSet
func (h *Handler) UpdateDaemonSet(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	var req struct {
		Image string `json:"image"`
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
	ds, err := client.Clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "daemonset not found")
		return
	}

	if req.Image != "" && len(ds.Spec.Template.Spec.Containers) > 0 {
		ds.Spec.Template.Spec.Containers[0].Image = req.Image
	}

	result, err := client.Clientset.AppsV1().DaemonSets(namespace).Update(ctx, ds, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// ==================== Job CRUD ====================

// GetJob 获取Job详情
func (h *Handler) GetJob(c *gin.Context) {
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
	job, err := client.Clientset.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "job not found")
		return
	}

	response.Success(c, job)
}

// CreateJob 创建Job
func (h *Handler) CreateJob(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		Namespace   string `json:"namespace" binding:"required"`
		Name        string `json:"name" binding:"required"`
		Image       string `json:"image" binding:"required"`
		Command     []string `json:"command"`
		Completions *int32 `json:"completions"`
		Parallelism *int32 `json:"parallelism"`
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

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: batchv1.JobSpec{
			Completions: req.Completions,
			Parallelism: req.Parallelism,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    req.Name,
							Image:   req.Image,
							Command: req.Command,
						},
					},
				},
			},
		},
	}

	ctx := context.Background()
	result, err := client.Clientset.BatchV1().Jobs(req.Namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

// ==================== CronJob CRUD ====================

// GetCronJob 获取CronJob详情
func (h *Handler) GetCronJob(c *gin.Context) {
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
	cj, err := client.Clientset.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "cronjob not found")
		return
	}

	response.Success(c, cj)
}

// CreateCronJob 创建CronJob
func (h *Handler) CreateCronJob(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}

	var req struct {
		Namespace string   `json:"namespace" binding:"required"`
		Name      string   `json:"name" binding:"required"`
		Schedule  string   `json:"schedule" binding:"required"`
		Image     string   `json:"image" binding:"required"`
		Command   []string `json:"command"`
		Suspend   bool     `json:"suspend"`
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

	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: req.Schedule,
			Suspend:  &req.Suspend,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{
									Name:    req.Name,
									Image:   req.Image,
									Command: req.Command,
								},
							},
						},
					},
				},
			},
		},
	}

	ctx := context.Background()
	result, err := client.Clientset.BatchV1().CronJobs(req.Namespace).Create(ctx, cj, metav1.CreateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, result)
}

// UpdateCronJob 更新CronJob
func (h *Handler) UpdateCronJob(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Param("ns")
	name := c.Param("name")

	var req struct {
		Schedule *string `json:"schedule"`
		Suspend  *bool   `json:"suspend"`
		Image    string  `json:"image"`
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
	cj, err := client.Clientset.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		response.NotFound(c, "cronjob not found")
		return
	}

	if req.Schedule != nil {
		cj.Spec.Schedule = *req.Schedule
	}
	if req.Suspend != nil {
		cj.Spec.Suspend = req.Suspend
	}
	if req.Image != "" && len(cj.Spec.JobTemplate.Spec.Template.Spec.Containers) > 0 {
		cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image = req.Image
	}

	result, err := client.Clientset.BatchV1().CronJobs(namespace).Update(ctx, cj, metav1.UpdateOptions{})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// ==================== Pod Update ====================

// UpdatePod 更新Pod（实际上只能删除重建，因为Pod不可变）
func (h *Handler) UpdatePod(c *gin.Context) {
	// Pod 不支持直接更新，返回提示
	response.BadRequest(c, "Pod is immutable, please delete and recreate it")
}

// ==================== CRD ====================

// ListCRDs 获取CRD列表
func (h *Handler) ListCRDs(c *gin.Context) {
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

	crds, err := client.Clientset.Discovery().ServerPreferredResources()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type CRDInfo struct {
		Group      string   `json:"group"`
		Version    string   `json:"version"`
		Kind       string   `json:"kind"`
		Name       string   `json:"name"`
		Namespaced bool     `json:"namespaced"`
		Verbs      []string `json:"verbs"`
	}

	result := make([]CRDInfo, 0)
	for _, list := range crds {
		if list == nil {
			continue
		}
		// 解析 GroupVersion (e.g., "policy.networking.k8s.io/v1alpha1")
		groupVersion := list.GroupVersion
		group := ""
		version := ""
		if idx := indexByte(groupVersion, '/'); idx >= 0 {
			group = groupVersion[:idx]
			version = groupVersion[idx+1:]
		} else {
			version = groupVersion
		}

		for _, resource := range list.APIResources {
			// 只显示CRD资源（通常是自定义资源）
			if len(group) > 0 && !isCoreResource(group) {
				result = append(result, CRDInfo{
					Group:      group,
					Version:    version,
					Kind:       resource.Kind,
					Name:       resource.Name,
					Namespaced: resource.Namespaced,
					Verbs:      resource.Verbs,
				})
			}
		}
	}

	response.Success(c, result)
}

// indexByte 查找字节在字符串中的位置
func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// isCoreResource 检查是否是核心资源
func isCoreResource(group string) bool {
	coreGroups := []string{
		"",
		"apps",
		"batch",
		"extensions",
		"networking.k8s.io",
		"storage.k8s.io",
		"rbac.authorization.k8s.io",
		"admissionregistration.k8s.io",
		"apiextensions.k8s.io",
		"autoscaling",
		"policy",
		"coordination.k8s.io",
		"discovery.k8s.io",
		"flowcontrol.apiserver.k8s.io",
		"node.k8s.io",
		"scheduling.k8s.io",
	}
	for _, g := range coreGroups {
		if group == g {
			return true
		}
	}
	return false
}

// ==================== Events ====================

// ListClusterEvents 获取集群事件
func (h *Handler) ListClusterEvents(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := c.Query("ns")
	limitStr := c.DefaultQuery("limit", "100")
	limit, _ := strconv.ParseInt(limitStr, 10, 64)

	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	ctx := context.Background()
	var events *corev1.EventList
	if namespace == "" {
		events, err = client.Clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{Limit: limit})
	} else {
		events, err = client.Clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{Limit: limit})
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
		Source    string `json:"source"`
		Age       string `json:"age"`
		Count     int32  `json:"count"`
	}

	result := make([]EventInfo, 0, len(events.Items))
	for _, e := range events.Items {
		result = append(result, EventInfo{
			Type:      e.Type,
			Reason:    e.Reason,
			Message:   e.Message,
			Namespace: e.Namespace,
			Object:    fmt.Sprintf("%s/%s", e.InvolvedObject.Kind, e.InvolvedObject.Name),
			Source:    e.Source.Component,
			Age:       timeSince(e.LastTimestamp.Time),
			Count:     e.Count,
		})
	}

	response.Success(c, result)
}
