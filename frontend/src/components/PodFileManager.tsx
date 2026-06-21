import { useState, useEffect } from 'react'
import {
  Modal, Table, Button, Space, message, Breadcrumb, Tooltip, Popconfirm, Upload
} from 'antd'
import {
  FolderOutlined, FileOutlined, EditOutlined, DeleteOutlined,
  DownloadOutlined, ArrowUpOutlined, ReloadOutlined, UploadOutlined
} from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { get, post, del } from '../api/request'

interface FileItem {
  name: string
  size: string
  permissions: string
  owner: string
  modified: string
  isDir: boolean
}

interface PodFileManagerProps {
  visible: boolean
  onClose: () => void
  clusterId: number
  namespace: string
  podName: string
  containerName?: string
}

const PodFileManager: React.FC<PodFileManagerProps> = ({
  visible,
  onClose,
  clusterId,
  namespace,
  podName,
  containerName,
}) => {
  const [currentPath, setCurrentPath] = useState('/')
  const [files, setFiles] = useState<FileItem[]>([])
  const [loading, setLoading] = useState(false)
  const [editVisible, setEditVisible] = useState(false)
  const [editFile, setEditFile] = useState<{ name: string; content: string } | null>(null)
  const [editContent, setEditContent] = useState('')

  useEffect(() => {
    if (visible) {
      fetchFiles('/')
    }
  }, [visible])

  const fetchFiles = async (path: string) => {
    setLoading(true)
    try {
      const res = await get<{ code: number; data: any }>(
        `/clusters/${clusterId}/workloads/pods/${namespace}/${podName}/files`,
        { params: { path, container: containerName } }
      )
      if (res.code === 0) {
        const lines = (res.data.output || '').split('\n').filter((l: string) => l.trim())
        const items: FileItem[] = []

        for (const line of lines) {
          const parts = line.split(/\s+/)
          if (parts.length < 9) continue
          const name = parts.slice(8).join(' ')
          // 跳过 . 和 ..
          if (name === '.' || name === '..') continue
          items.push({
            permissions: parts[0],
            owner: parts[2] + ' ' + parts[3],
            size: parts[4],
            modified: parts.slice(5, 8).join(' '),
            name: name,
            isDir: parts[0].startsWith('d'),
          })
        }

        // 排序：目录在前，文件在后
        items.sort((a, b) => {
          if (a.isDir && !b.isDir) return -1
          if (!a.isDir && b.isDir) return 1
          return a.name.localeCompare(b.name)
        })

        setFiles(items)
        setCurrentPath(path)
      } else {
        message.error('获取文件列表失败')
      }
    } catch (e) {
      console.error(e)
      message.error('获取文件列表失败')
    } finally {
      setLoading(false)
    }
  }

  const handleNavigate = (name: string) => {
    const newPath = currentPath === '/' ? `/${name}` : `${currentPath}/${name}`
    fetchFiles(newPath)
  }

  const handleUp = () => {
    const parentPath = currentPath.substring(0, currentPath.lastIndexOf('/')) || '/'
    fetchFiles(parentPath)
  }

  const handleReadFile = async (name: string) => {
    const filePath = currentPath === '/' ? `/${name}` : `${currentPath}/${name}`
    try {
      const res = await get<{ code: number; data: any }>(
        `/clusters/${clusterId}/workloads/pods/${namespace}/${podName}/files/read`,
        { params: { path: filePath, container: containerName } }
      )
      if (res.code === 0) {
        setEditFile({ name, content: res.data.content || '' })
        setEditContent(res.data.content || '')
        setEditVisible(true)
      }
    } catch (e) {
      message.error('读取文件失败')
    }
  }

  const handleSaveFile = async () => {
    if (!editFile) return
    const filePath = currentPath === '/' ? `/${editFile.name}` : `${currentPath}/${editFile.name}`
    try {
      await post(`/clusters/${clusterId}/workloads/pods/${namespace}/${podName}/files/write`, {
        container: containerName,
        path: filePath,
        content: editContent,
      })
      message.success('保存成功')
      setEditVisible(false)
    } catch (e) {
      message.error('保存失败')
    }
  }

  const handleDelete = async (name: string) => {
    const filePath = currentPath === '/' ? `/${name}` : `${currentPath}/${name}`
    try {
      await del(`/clusters/${clusterId}/workloads/pods/${namespace}/${podName}/files`, {
        params: { path: filePath, container: containerName },
      })
      message.success('删除成功')
      fetchFiles(currentPath)
    } catch (e) {
      message.error('删除失败')
    }
  }

  const handleDownload = (name: string) => {
    const filePath = currentPath === '/' ? `/${name}` : `${currentPath}/${name}`
    const url = `/api/v1/clusters/${clusterId}/workloads/pods/${namespace}/${podName}/files/download?path=${encodeURIComponent(filePath)}&container=${containerName || ''}`
    window.open(url, '_blank')
  }

  const handleUpload = async (file: File) => {
    const reader = new FileReader()
    reader.onload = async (e) => {
      const content = e.target?.result as string
      const filePath = currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`
      try {
        await post(`/clusters/${clusterId}/workloads/pods/${namespace}/${podName}/files/write`, {
          container: containerName,
          path: filePath,
          content: content,
        })
        message.success(`文件 ${file.name} 上传成功`)
        fetchFiles(currentPath)
      } catch (e) {
        message.error('上传失败')
      }
    }
    reader.readAsText(file)
    return false // 阻止默认上传行为
  }

  const columns: ColumnsType<FileItem> = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, record: FileItem) => (
        <Space>
          {record.isDir ? (
            <FolderOutlined style={{ color: '#faad14' }} />
          ) : (
            <FileOutlined style={{ color: '#1890ff' }} />
          )}
          {record.isDir ? (
            <a onClick={() => handleNavigate(name)} style={{ cursor: 'pointer' }}>{name}</a>
          ) : (
            <span>{name}</span>
          )}
        </Space>
      ),
    },
    { title: '大小', dataIndex: 'size', key: 'size', width: 100 },
    { title: '权限', dataIndex: 'permissions', key: 'permissions', width: 120 },
    { title: '所有者', dataIndex: 'owner', key: 'owner', width: 150 },
    { title: '修改时间', dataIndex: 'modified', key: 'modified', width: 150 },
    {
      title: '操作',
      key: 'action',
      width: 200,
      render: (_, record) => {
        if (record.isDir) return null
        return (
          <Space size="small">
            <Tooltip title="编辑">
              <Button type="link" icon={<EditOutlined />} onClick={() => handleReadFile(record.name)} />
            </Tooltip>
            <Tooltip title="下载">
              <Button type="link" icon={<DownloadOutlined />} onClick={() => handleDownload(record.name)} />
            </Tooltip>
            <Popconfirm title="确定删除？" onConfirm={() => handleDelete(record.name)}>
              <Tooltip title="删除">
                <Button type="link" danger icon={<DeleteOutlined />} />
              </Tooltip>
            </Popconfirm>
          </Space>
        )
      },
    },
  ]

  const pathParts = currentPath.split('/').filter(Boolean)

  return (
    <>
      <Modal
        title={`文件管理 - ${podName}`}
        open={visible}
        onCancel={onClose}
        footer={null}
        width={1000}
      >
        <div style={{ marginBottom: 16 }}>
          <Space>
            <Button icon={<ArrowUpOutlined />} onClick={handleUp} disabled={currentPath === '/'}>
              上级目录
            </Button>
            <Button icon={<ReloadOutlined />} onClick={() => fetchFiles(currentPath)}>
              刷新
            </Button>
            <Upload
              showUploadList={false}
              beforeUpload={handleUpload}
              accept="*"
            >
              <Button icon={<UploadOutlined />}>
                上传文件
              </Button>
            </Upload>
            <Breadcrumb
              items={[
                { title: <a onClick={() => fetchFiles('/')}>/</a> },
                ...pathParts.map((part, index) => ({
                  title: <a onClick={() => fetchFiles('/' + pathParts.slice(0, index + 1).join('/'))}>{part}</a>,
                })),
              ]}
            />
          </Space>
        </div>

        <Table
          columns={columns}
          dataSource={files}
          rowKey="name"
          loading={loading}
          pagination={false}
          size="small"
          scroll={{ y: 400 }}
        />
      </Modal>

      {/* Edit File Modal */}
      <Modal
        title={`编辑文件: ${editFile?.name}`}
        open={editVisible}
        onCancel={() => setEditVisible(false)}
        onOk={handleSaveFile}
        width={800}
      >
        <textarea
          value={editContent}
          onChange={(e) => setEditContent(e.target.value)}
          style={{
            width: '100%',
            height: 400,
            fontFamily: 'Consolas, Monaco, monospace',
            fontSize: 13,
            padding: 16,
            border: '1px solid #d9d9d9',
            borderRadius: 8,
            background: '#f5f5f5',
          }}
          spellCheck={false}
        />
      </Modal>
    </>
  )
}

export default PodFileManager
