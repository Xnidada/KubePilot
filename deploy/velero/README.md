# KubePilot 备份依赖：安装 Velero

KubePilot 的「备份管理」会在目标集群创建真实的 `velero.io/v1` **Backup / Restore** 对象。  
若集群中没有 Velero CRD，创建/恢复接口会直接返回 400，**不会再假成功**。

本目录提供一套**开发/单机**可用的安装模板：

| 文件 | 说明 |
|------|------|
| `minio.yaml` | MinIO（S3 兼容）+ 初始化 bucket `kubepilot` |
| `values-minio.yaml` | Velero Helm values（对接同命名空间 MinIO） |
| `credentials-velero.example` | AWS 风格凭证示例（与 MinIO 默认账号一致） |
| `install.sh` | 一键安装 / 卸载 / 校验 |

> 生产环境请将 MinIO 换成 OSS/S3/COS 等，并修改 `values-minio.yaml` 中的 BSL 与凭证。

## 前置条件

- `kubectl` 已指向目标集群，且有集群管理权限
- `helm` v3
- 集群可拉取镜像：`minio/minio`、`minio/mc`、`velero/velero`、`velero/velero-plugin-for-aws`

## 快速安装

```bash
# 在仓库根目录执行
chmod +x deploy/velero/install.sh
./deploy/velero/install.sh install
```

无默认 StorageClass 时，脚本会自动改用 hostPath（默认 `/opt/kubepilot/minio-data`）：

```bash
MINIO_HOSTPATH=/data/minio ./deploy/velero/install.sh install
```

### 网络受限（拉不下 GitHub Chart）

在可联网机器上预先拉取 Chart，再拷到目标机离线安装：

```bash
helm repo add vmware-tanzu https://vmware-tanzu.github.io/helm-charts
helm pull vmware-tanzu/velero --untar -d /tmp/velero-chart
# 将 /tmp/velero-chart/velero 同步到目标机后：
VELERO_CHART_PATH=/path/to/velero ./deploy/velero/install.sh install
```

## 校验

```bash
./deploy/velero/install.sh verify

# CRD
kubectl get crd | grep velero.io

# BSL 状态应为 Available
kubectl -n velero get backupstoragelocations.velero.io
```

在 KubePilot UI：**系统 → 备份管理**，告警条应消失；或：

```bash
curl -H "Authorization: Bearer <token>" \
  'http://<kubepilot>/api/v1/backups/capability?cluster_id=1'
# 期望: "velero_available": true
```

## 与 KubePilot 的关系

1. KubePilot 探测 `velero.io/v1` 的 `backups` 资源是否存在  
2. 创建备份时，在 `velero` 命名空间写入 `Backup` CR（见 `internal/handler/backup/velero.go`）  
3. 恢复时写入 `Restore` CR，并等待终端 phase  

当前模板默认**不启用** Node-Agent / 卷快照，适合先跑通「资源对象备份」。若需 PV 文件级备份，请在 `values-minio.yaml` 中打开 `deployNodeAgent` 并按 Velero 文档调整。

## 卸载

```bash
./deploy/velero/install.sh uninstall
```

卸载脚本默认保留 CRD。若需彻底清理：

```bash
kubectl get crd | awk '/velero.io/ {print $1}' | xargs -r kubectl delete crd
```

## 常见问题

**BSL 一直 Unavailable**  
- 检查 MinIO：`kubectl -n velero get po,svc`  
- 检查凭证是否与 MinIO Secret 一致  
- `kubectl -n velero logs deploy/velero`

**KubePilot 仍提示未安装 Velero**  
- 确认操作的是「目标集群」而非仅本机 kubeconfig 搞混  
- `kubectl get crd backups.velero.io`
