package cluster

import (
	"context"
	"errors"
	"fmt"

	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/model"
	"github.com/kubepilot/kubepilot/internal/pkg/crypto"
	"github.com/kubepilot/kubepilot/internal/repository"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Service struct {
	clusterRepo *repository.ClusterRepository
	encryptKey  string
}

func NewService(db *gorm.DB, encryptKey string) *Service {
	return &Service{
		clusterRepo: repository.NewClusterRepository(db),
		encryptKey:  encryptKey,
	}
}

type CreateClusterRequest struct {
	Name        string `json:"name" binding:"required,min=3,max=128"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	APIServer   string `json:"api_server" binding:"required"`
	Kubeconfig  string `json:"kubeconfig" binding:"required"`
	Tags        string `json:"tags"`
}

type UpdateClusterRequest struct {
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Tags        string `json:"tags"`
}

type ClusterResponse struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	DisplayName    string `json:"display_name"`
	Description    string `json:"description"`
	APIServer      string `json:"api_server"`
	Status         string `json:"status"`
	Version        string `json:"version"`
	NodeCount      int    `json:"node_count"`
	CPUCapacity    string `json:"cpu_capacity"`
	MemoryCapacity string `json:"memory_capacity"`
	LastHealthCheck *string `json:"last_health_check"`
	Tags           string `json:"tags"`
	CreatedAt      string `json:"created_at"`
}

func (s *Service) Create(req *CreateClusterRequest) (*ClusterResponse, error) {
	// Check if cluster name exists
	_, err := s.clusterRepo.GetByName(req.Name)
	if err == nil {
		return nil, errors.New("cluster name already exists")
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// Encrypt kubeconfig
	encryptedConfig, err := crypto.Encrypt(req.Kubeconfig, s.encryptKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt kubeconfig: %w", err)
	}

	cluster := &model.Cluster{
		Name:       req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		APIServer:   req.APIServer,
		Kubeconfig:  encryptedConfig,
		Status:      "unknown",
		Tags:        req.Tags,
	}

	if err := s.clusterRepo.Create(cluster); err != nil {
		return nil, err
	}

	// Try to register client
	go s.registerClient(cluster)

	return s.toResponse(cluster), nil
}

func (s *Service) GetByID(id uint) (*ClusterResponse, error) {
	cluster, err := s.clusterRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	return s.toResponse(cluster), nil
}

func (s *Service) Update(id uint, req *UpdateClusterRequest) (*ClusterResponse, error) {
	cluster, err := s.clusterRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	if req.DisplayName != "" {
		cluster.DisplayName = req.DisplayName
	}
	if req.Description != "" {
		cluster.Description = req.Description
	}
	if req.Tags != "" {
		cluster.Tags = req.Tags
	}

	if err := s.clusterRepo.Update(cluster); err != nil {
		return nil, err
	}

	return s.toResponse(cluster), nil
}

func (s *Service) Delete(id uint) error {
	cluster, err := s.clusterRepo.GetByID(id)
	if err != nil {
		return err
	}

	// Remove client from manager
	k8s.Manager.RemoveClient(cluster.ID)

	return s.clusterRepo.Delete(id)
}

func (s *Service) List(page, size int) ([]ClusterResponse, int64, error) {
	clusters, total, err := s.clusterRepo.List(page, size)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]ClusterResponse, len(clusters))
	for i, cluster := range clusters {
		responses[i] = *s.toResponse(&cluster)
	}

	return responses, total, nil
}

func (s *Service) HealthCheck(id uint) error {
	cluster, err := s.clusterRepo.GetByID(id)
	if err != nil {
		return err
	}

	// Decrypt kubeconfig
	kubeconfig, err := crypto.Decrypt(cluster.Kubeconfig, s.encryptKey)
	if err != nil {
		s.clusterRepo.UpdateHealthCheckError(id, "failed to decrypt kubeconfig")
		return err
	}

	// Register client if not exists
	if _, err := k8s.Manager.GetClient(id); err != nil {
		if err := k8s.Manager.RegisterClient(id, []byte(kubeconfig), "default"); err != nil {
			s.clusterRepo.UpdateHealthCheckError(id, err.Error())
			return err
		}
	}

	// Get cluster info
	info, err := k8s.Manager.GetClusterInfo(id)
	if err != nil {
		s.clusterRepo.UpdateHealthCheckError(id, err.Error())
		return err
	}

	// Update cluster info
	return s.clusterRepo.UpdateHealthCheck(id, "connected", info.Version, info.NodeCount, info.CPUCapacity, info.MemCapacity)
}

func (s *Service) GetClusterInfo(id uint) (*k8s.ClusterInfo, error) {
	return k8s.Manager.GetClusterInfo(id)
}

func (s *Service) GetNamespaces(id uint) ([]string, error) {
	client, err := k8s.Manager.GetClient(id)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	namespaces, err := client.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	result := make([]string, len(namespaces.Items))
	for i, ns := range namespaces.Items {
		result[i] = ns.Name
	}

	return result, nil
}

func (s *Service) GetNodes(id uint) ([]k8s.NodeInfo, error) {
	info, err := k8s.Manager.GetClusterInfo(id)
	if err != nil {
		return nil, err
	}
	return info.Nodes, nil
}

func (s *Service) registerClient(cluster *model.Cluster) {
	kubeconfig, err := crypto.Decrypt(cluster.Kubeconfig, s.encryptKey)
	if err != nil {
		return
	}

	k8s.Manager.RegisterClient(cluster.ID, []byte(kubeconfig), "default")
}

func (s *Service) toResponse(cluster *model.Cluster) *ClusterResponse {
	resp := &ClusterResponse{
		ID:             cluster.ID,
		Name:           cluster.Name,
		DisplayName:    cluster.DisplayName,
		Description:    cluster.Description,
		APIServer:      cluster.APIServer,
		Status:         cluster.Status,
		Version:        cluster.Version,
		NodeCount:      cluster.NodeCount,
		CPUCapacity:    cluster.CPUCapacity,
		MemoryCapacity: cluster.MemoryCapacity,
		Tags:           cluster.Tags,
		CreatedAt:      cluster.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	if cluster.LastHealthCheck != nil {
		ts := cluster.LastHealthCheck.Format("2006-01-02 15:04:05")
		resp.LastHealthCheck = &ts
	}

	return resp
}
