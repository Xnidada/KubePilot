package workload

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubepilot/kubepilot/internal/authz"
	"github.com/kubepilot/kubepilot/internal/k8s"
	"github.com/kubepilot/kubepilot/internal/pkg/response"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const gatewayAPIGroup = "gateway.networking.k8s.io"

var gatewayAPIResources = []struct{ kind, resource string }{
	{"GatewayClass", "gatewayclasses"},
	{"Gateway", "gateways"},
	{"HTTPRoute", "httproutes"},
	{"GRPCRoute", "grpcroutes"},
	{"ReferenceGrant", "referencegrants"},
}

type gatewayAPIItem struct {
	Kind       string                 `json:"kind"`
	Name       string                 `json:"name"`
	Namespace  string                 `json:"namespace,omitempty"`
	Generation int64                  `json:"generation"`
	Spec       map[string]interface{} `json:"spec"`
	Status     map[string]interface{} `json:"status"`
}

type gatewayAPIOverview struct {
	Installed       bool              `json:"installed"`
	CRDsReady       bool              `json:"crds_ready"`
	EnvoyController string            `json:"envoy_controller"`
	Versions        map[string]string `json:"versions"`
	ClassesVisible  bool              `json:"classes_visible"`
	Items           []gatewayAPIItem  `json:"items"`
}

// ListGatewayAPI is deliberately read-only and exposes only the named Gateway API kinds.
func (h *Handler) ListGatewayAPI(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || clusterID == 0 {
		response.BadRequest(c, "invalid cluster id")
		return
	}
	namespace := strings.TrimSpace(c.Query("ns"))
	if namespace != "" && len(validation.IsDNS1123Label(namespace)) != 0 {
		response.BadRequest(c, "invalid namespace")
		return
	}
	client, err := k8s.Manager.GetClient(uint(clusterID))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	result := gatewayAPIOverview{Versions: map[string]string{}, Items: []gatewayAPIItem{}}
	for _, version := range []string{"v1", "v1beta1"} {
		resources, err := client.Clientset.Discovery().ServerResourcesForGroupVersion(gatewayAPIGroup + "/" + version)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			response.InternalError(c, "Gateway API discovery failed: "+err.Error())
			return
		}
		for _, resource := range resources.APIResources {
			for _, supported := range gatewayAPIResources {
				if resource.Name == supported.resource && result.Versions[supported.kind] == "" {
					result.Versions[supported.kind] = version
				}
			}
		}
	}
	result.Installed = len(result.Versions) > 0
	result.CRDsReady = result.Versions["GatewayClass"] != "" && result.Versions["Gateway"] != "" && result.Versions["HTTPRoute"] != ""
	result.EnvoyController = gatewayEnvoyControllerStatus(ctx, client.Clientset)
	if !result.Installed {
		response.Success(c, result)
		return
	}
	dynamicClient, err := dynamic.NewForConfig(client.Config)
	if err != nil {
		response.InternalError(c, "Gateway API client unavailable: "+err.Error())
		return
	}
	result.ClassesVisible = authz.RequireScope(c, "custom_resources", "view", uint(clusterID), "*") == nil
	namespaces := gatewayQueryNamespaces(namespace, authz.AllowedNamespaceSet(c))
	for _, resource := range gatewayAPIResources {
		version := result.Versions[resource.kind]
		if version == "" || (resource.kind == "GatewayClass" && !result.ClassesVisible) {
			continue
		}
		scopes := namespaces
		if resource.kind == "GatewayClass" {
			scopes = []string{""}
		}
		items, err := listGatewayAPIItems(ctx, dynamicClient, schema.GroupVersionResource{
			Group: gatewayAPIGroup, Version: version, Resource: resource.resource,
		}, resource.kind, scopes)
		if err != nil {
			response.InternalError(c, fmt.Sprintf("list %s failed: %v", resource.kind, err))
			return
		}
		result.Items = append(result.Items, items...)
	}
	response.Success(c, result)
}

// This reports only Envoy Gateway; other controllers are represented by GatewayClass conditions.
func gatewayEnvoyControllerStatus(ctx context.Context, client kubernetes.Interface) string {
	deployment, err := client.AppsV1().Deployments(gatewayInstallerNamespace).Get(ctx, "envoy-gateway", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "absent"
	}
	if err != nil {
		return "unknown"
	}
	if deployment.Status.AvailableReplicas > 0 && deployment.Status.ObservedGeneration >= deployment.Generation {
		return "ready"
	}
	return "not_ready"
}

func gatewayQueryNamespaces(selected string, allowed map[string]struct{}) []string {
	if selected != "" {
		return []string{selected}
	}
	if allowed == nil {
		return []string{""}
	}
	namespaces := make([]string, 0, len(allowed))
	for namespace := range allowed {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)
	return namespaces
}

func listGatewayAPIItems(ctx context.Context, client dynamic.Interface, gvr schema.GroupVersionResource, kind string, namespaces []string) ([]gatewayAPIItem, error) {
	items := make([]gatewayAPIItem, 0)
	for _, namespace := range namespaces {
		var resource dynamic.ResourceInterface = client.Resource(gvr)
		if namespace != "" {
			resource = client.Resource(gvr).Namespace(namespace)
		}
		continuation := ""
		for {
			list, err := resource.List(ctx, metav1.ListOptions{Limit: 250, Continue: continuation})
			if err != nil {
				return nil, err
			}
			for _, object := range list.Items {
				items = append(items, gatewayAPIItemFromObject(kind, object))
				if len(items) > 5000 {
					// ponytail: cap one response; add server-side paging if larger clusters need it.
					return nil, fmt.Errorf("too many %s resources; select one namespace", kind)
				}
			}
			continuation = list.GetContinue()
			if continuation == "" {
				break
			}
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Namespace == items[j].Namespace {
			return items[i].Name < items[j].Name
		}
		return items[i].Namespace < items[j].Namespace
	})
	return items, nil
}

func gatewayAPIItemFromObject(kind string, object unstructured.Unstructured) gatewayAPIItem {
	spec, _, _ := unstructured.NestedMap(object.Object, "spec")
	status, _, _ := unstructured.NestedMap(object.Object, "status")
	return gatewayAPIItem{
		Kind: kind, Name: object.GetName(), Namespace: object.GetNamespace(),
		Generation: object.GetGeneration(), Spec: spec, Status: status,
	}
}
