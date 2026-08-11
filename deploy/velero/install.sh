#!/usr/bin/env bash
# 在目标 Kubernetes 集群安装 MinIO + Velero，供 KubePilot 备份模块使用。
# 用法:
#   ./deploy/velero/install.sh              # 默认安装
#   ./deploy/velero/install.sh uninstall    # 卸载
#   MINIO_HOSTPATH=/data/minio ./deploy/velero/install.sh   # 无 StorageClass 时用 hostPath
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NS="${VELERO_NAMESPACE:-velero}"
RELEASE="${VELERO_RELEASE:-velero}"
CHART_REPO_NAME="${VELERO_CHART_REPO_NAME:-vmware-tanzu}"
CHART_REPO_URL="${VELERO_CHART_REPO_URL:-https://vmware-tanzu.github.io/helm-charts}"
CHART="${VELERO_CHART:-vmware-tanzu/velero}"
# 离线/网络受限时：先在可联网机器 helm pull，再设置
#   VELERO_CHART_PATH=/path/to/velero  ./install.sh
CHART_PATH="${VELERO_CHART_PATH:-}"
VALUES_FILE="${VELERO_VALUES:-$ROOT_DIR/values-minio.yaml}"
MINIO_MANIFEST="${MINIO_MANIFEST:-$ROOT_DIR/minio.yaml}"
MINIO_HOSTPATH="${MINIO_HOSTPATH:-/opt/kubepilot/minio-data}"
MODE="${1:-install}"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "缺少命令: $1" >&2
    exit 1
  }
}

need kubectl
need helm

has_default_sc() {
  local name
  name="$(kubectl get storageclass -o jsonpath='{range .items[?(@.metadata.annotations.storageclass\.kubernetes\.io/is-default-class=="true")]}{.metadata.name}{end}' 2>/dev/null || true)"
  [[ -n "$name" ]]
}

apply_minio() {
  echo "==> 安装 MinIO (namespace=$NS)"
  kubectl delete job -n "$NS" minio-init-bucket --ignore-not-found=true >/dev/null 2>&1 || true
  if has_default_sc; then
    echo "检测到默认 StorageClass，使用 PVC"
    kubectl apply -f "$MINIO_MANIFEST"
  else
    echo "未检测到默认 StorageClass，改用 hostPath: $MINIO_HOSTPATH"
    mkdir -p "$MINIO_HOSTPATH"
    python3 - "$MINIO_MANIFEST" "$MINIO_HOSTPATH" <<'PY' | kubectl apply -f -
import sys, re
src, hostpath = sys.argv[1], sys.argv[2]
text = open(src, encoding="utf-8").read()
parts = re.split(r"(?m)^---\s*$", text)
kept = []
for p in parts:
    if "kind: PersistentVolumeClaim" in p and "minio-data" in p:
        continue
    kept.append(p)
text = "---\n".join(kept)
text = text.replace(
    "          persistentVolumeClaim:\n            claimName: minio-data\n",
    f"          hostPath:\n            path: {hostpath}\n            type: DirectoryOrCreate\n",
)
print(text)
PY
  fi

  echo "==> 等待 MinIO Ready"
  kubectl -n "$NS" rollout status deploy/minio --timeout=180s
  echo "==> 等待 bucket 初始化 Job"
  kubectl -n "$NS" wait --for=condition=complete job/minio-init-bucket --timeout=180s \
    || { echo "Job 未完成，打印日志:"; kubectl -n "$NS" logs job/minio-init-bucket || true; exit 1; }
}

install_velero() {
  local chart_ref="$CHART"
  if [[ -n "$CHART_PATH" ]]; then
    if [[ ! -d "$CHART_PATH" && ! -f "$CHART_PATH" ]]; then
      echo "VELERO_CHART_PATH 无效: $CHART_PATH" >&2
      exit 1
    fi
    chart_ref="$CHART_PATH"
    echo "==> 使用本地 Chart: $chart_ref"
  else
    echo "==> 添加/更新 Helm repo: $CHART_REPO_NAME"
    if ! helm repo list 2>/dev/null | awk '{print $1}' | grep -qx "$CHART_REPO_NAME"; then
      helm repo add "$CHART_REPO_NAME" "$CHART_REPO_URL"
    fi
    helm repo update "$CHART_REPO_NAME" >/dev/null
  fi

  echo "==> Helm 安装/升级 Velero ($RELEASE)"
  # 先应用 CRD，避免依赖 chart 的 upgradeCRDs Job
  if [[ -d "$chart_ref/crds" ]]; then
    echo "==> 预先 apply CRDs from $chart_ref/crds"
    kubectl apply -f "$chart_ref/crds"
  elif [[ -f "$chart_ref" && "$chart_ref" == *.tgz ]]; then
    tmp="$(mktemp -d)"
    tar -xzf "$chart_ref" -C "$tmp"
    if [[ -d "$tmp/velero/crds" ]]; then
      kubectl apply -f "$tmp/velero/crds"
    fi
    rm -rf "$tmp"
  fi

  helm upgrade --install "$RELEASE" "$chart_ref" \
    --namespace "$NS" \
    --create-namespace \
    -f "$VALUES_FILE" \
    --wait \
    --timeout 10m

  echo "==> 等待 Velero Deployment"
  kubectl -n "$NS" rollout status deploy/velero --timeout=180s || \
    kubectl -n "$NS" rollout status deployment -l app.kubernetes.io/name=velero --timeout=180s || true

  echo "==> 校验 CRD"
  kubectl get crd backups.velero.io restores.velero.io backupstoragelocations.velero.io
  kubectl -n "$NS" get backupstoragelocations.velero.io -o wide || true
}

uninstall_all() {
  echo "==> 卸载 Velero Helm release"
  helm uninstall "$RELEASE" -n "$NS" 2>/dev/null || true
  echo "==> 删除 MinIO / namespace（保留 CRD，避免误伤其他安装）"
  kubectl delete -f "$MINIO_MANIFEST" --ignore-not-found=true 2>/dev/null || true
  kubectl delete ns "$NS" --ignore-not-found=true --wait=false || true
  echo "完成。如需删除 CRD: kubectl get crd | grep velero.io | awk '{print \$1}' | xargs -r kubectl delete crd"
}

verify() {
  echo "==> 验证"
  kubectl get crd | grep velero.io || {
    echo "Velero CRD 未找到" >&2
    exit 1
  }
  kubectl -n "$NS" get deploy,po,svc
  echo
  echo "KubePilot 侧：打开「系统 / 备份管理」，能力探测应为 velero_available=true"
  echo "或调用: GET /api/v1/backups/capability?cluster_id=<id>"
}

case "$MODE" in
  install)
    apply_minio
    install_velero
    verify
    echo
    echo "安装完成。"
    ;;
  uninstall)
    uninstall_all
    ;;
  verify)
    verify
    ;;
  *)
    echo "用法: $0 [install|uninstall|verify]" >&2
    exit 1
    ;;
esac
