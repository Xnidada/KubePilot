package repository

import (
	"github.com/kubepilot/kubepilot/internal/model"
	"gorm.io/gorm"
)

type ClusterRepository struct {
	db *gorm.DB
}

func NewClusterRepository(db *gorm.DB) *ClusterRepository {
	return &ClusterRepository{db: db}
}

func (r *ClusterRepository) Create(cluster *model.Cluster) error {
	return r.db.Create(cluster).Error
}

func (r *ClusterRepository) GetByID(id uint) (*model.Cluster, error) {
	var cluster model.Cluster
	err := r.db.First(&cluster, id).Error
	if err != nil {
		return nil, err
	}
	return &cluster, nil
}

func (r *ClusterRepository) GetByName(name string) (*model.Cluster, error) {
	var cluster model.Cluster
	// Check including soft-deleted records to prevent name conflict
	err := r.db.Unscoped().Where("name = ? AND deleted_at IS NULL", name).First(&cluster).Error
	if err != nil {
		return nil, err
	}
	return &cluster, nil
}

func (r *ClusterRepository) Update(cluster *model.Cluster) error {
	return r.db.Save(cluster).Error
}

func (r *ClusterRepository) Delete(id uint) error {
	// Hard delete to allow name reuse
	return r.db.Unscoped().Delete(&model.Cluster{}, id).Error
}

func (r *ClusterRepository) List(page, size int) ([]model.Cluster, int64, error) {
	var clusters []model.Cluster
	var total int64

	r.db.Model(&model.Cluster{}).Count(&total)
	err := r.db.Offset((page - 1) * size).Limit(size).Order("id desc").Find(&clusters).Error
	if err != nil {
		return nil, 0, err
	}

	return clusters, total, nil
}

func (r *ClusterRepository) ListAll() ([]model.Cluster, error) {
	var clusters []model.Cluster
	err := r.db.Order("name").Find(&clusters).Error
	return clusters, err
}

func (r *ClusterRepository) UpdateStatus(id uint, status string) error {
	return r.db.Model(&model.Cluster{}).Where("id = ?", id).Update("status", status).Error
}

func (r *ClusterRepository) UpdateHealthCheck(id uint, status, version string, nodeCount int, cpuCapacity, memCapacity string) error {
	updates := map[string]interface{}{
		"status":               status,
		"version":              version,
		"node_count":           nodeCount,
		"cpu_capacity":         cpuCapacity,
		"memory_capacity":      memCapacity,
		"last_health_check":    gorm.Expr("NOW()"),
		"last_health_check_error": "",
	}
	return r.db.Model(&model.Cluster{}).Where("id = ?", id).Updates(updates).Error
}

func (r *ClusterRepository) UpdateHealthCheckError(id uint, errMessage string) error {
	updates := map[string]interface{}{
		"status":                  "error",
		"last_health_check":       gorm.Expr("NOW()"),
		"last_health_check_error": errMessage,
	}
	return r.db.Model(&model.Cluster{}).Where("id = ?", id).Updates(updates).Error
}
