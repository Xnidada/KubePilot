import { useEffect, useState } from 'react'
import { Modal, Table, Tag, Button, Space, message, Popconfirm, Typography } from 'antd'
import { RollbackOutlined, HistoryOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { getDeploymentHistory, rollbackDeployment } from '../api/workload'

const { Text } = Typography

interface RevisionInfo {
  revision: number
  replicas: number
  create_time: string
  images: string
}

interface DeploymentHistoryModalProps {
  visible: boolean
  onClose: () => void
  onSuccess: () => void
  clusterId: number
  namespace: string
  name: string
}

const DeploymentHistoryModal: React.FC<DeploymentHistoryModalProps> = ({
  visible,
  onClose,
  onSuccess,
  clusterId,
  namespace,
  name,
}) => {
  const [loading, setLoading] = useState(false)
  const [revisions, setRevisions] = useState<RevisionInfo[]>([])
  const [currentRevision, setCurrentRevision] = useState<string>('')

  useEffect(() => {
    if (visible) {
      fetchHistory()
    }
  }, [visible])

  const fetchHistory = async () => {
    setLoading(true)
    try {
      const res = await getDeploymentHistory(clusterId, namespace, name)
      if (res.code === 0) {
        setRevisions(res.data.revisions || [])
        setCurrentRevision(res.data.current_revision || '')
      }
    } catch (error) {
      console.error('Failed to fetch history:', error)
      message.error('获取历史记录失败')
    } finally {
      setLoading(false)
    }
  }

  const handleRollback = async (revision: number) => {
    try {
      await rollbackDeployment(clusterId, namespace, name, revision)
      message.success(`回滚到版本 ${revision} 成功`)
      onSuccess()
      onClose()
    } catch (error) {
      console.error('Rollback failed:', error)
      message.error('回滚失败')
    }
  }

  const columns: ColumnsType<RevisionInfo> = [
    {
      title: '版本',
      dataIndex: 'revision',
      key: 'revision',
      render: (revision) => (
        <Space>
          <Text strong>{revision}</Text>
          {String(revision) === currentRevision && <Tag color="blue">当前</Tag>}
        </Space>
      ),
    },
    {
      title: '副本数',
      dataIndex: 'replicas',
      key: 'replicas',
    },
    {
      title: '镜像',
      dataIndex: 'images',
      key: 'images',
      ellipsis: true,
    },
    {
      title: '创建时间',
      dataIndex: 'create_time',
      key: 'create_time',
    },
    {
      title: '操作',
      key: 'action',
      width: 100,
      render: (_, record) => (
        String(record.revision) !== currentRevision ? (
          <Popconfirm
            title={`确定要回滚到版本 ${record.revision} 吗？`}
            onConfirm={() => handleRollback(record.revision)}
            okText="确定"
            cancelText="取消"
          >
            <Button type="link" icon={<RollbackOutlined />}>
              回滚
            </Button>
          </Popconfirm>
        ) : (
          <Text type="secondary">当前版本</Text>
        )
      ),
    },
  ]

  return (
    <Modal
      title={
        <Space>
          <HistoryOutlined />
          <span>修订历史 - {name}</span>
        </Space>
      }
      open={visible}
      onCancel={onClose}
      footer={null}
      width={800}
    >
      <Table
        columns={columns}
        dataSource={revisions}
        rowKey="revision"
        loading={loading}
        pagination={false}
        size="small"
      />
    </Modal>
  )
}

export default DeploymentHistoryModal
