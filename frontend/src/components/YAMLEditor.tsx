import { useState, useEffect } from 'react'
import { Modal, Button, Space, message, Spin } from 'antd'
import { SaveOutlined, CopyOutlined, ReloadOutlined } from '@ant-design/icons'

interface YAMLEditorProps {
  visible: boolean
  onClose: () => void
  onSuccess: () => void
  clusterId: number
  resourceType: string
  namespace: string
  resourceName: string
}

const YAMLEditor: React.FC<YAMLEditorProps> = ({
  visible,
  onClose,
  onSuccess,
  clusterId,
  resourceType,
  namespace,
  resourceName,
}) => {
  const [yaml, setYaml] = useState('')
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (visible) {
      fetchYAML()
    }
  }, [visible])

  const fetchYAML = async () => {
    setLoading(true)
    try {
      const token = getAuthToken()
      const nsPath = namespace ? `/${namespace}` : '/_'
      const response = await fetch(
        `/api/v1/clusters/${clusterId}/workloads/yaml/${resourceType}${nsPath}/${resourceName}`,
        { headers: { 'Authorization': `Bearer ${token}` } }
      )
      const res = await response.json()
      if (res.code === 0) {
        setYaml(res.data.yaml || '')
      } else {
        message.error('获取 YAML 失败')
      }
    } catch (error) {
      message.error('获取 YAML 失败')
    } finally {
      setLoading(false)
    }
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      const token = getAuthToken()
      const response = await fetch(
        `/api/v1/clusters/${clusterId}/workloads/yaml/apply`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`,
          },
          body: JSON.stringify({ yaml }),
        }
      )
      const res = await response.json()
      if (res.code === 0) {
        message.success('更新成功')
        onSuccess()
        onClose()
      } else {
        message.error(res.message || '更新失败')
      }
    } catch (error) {
      message.error('更新失败')
    } finally {
      setSaving(false)
    }
  }

  const handleCopy = () => {
    navigator.clipboard.writeText(yaml)
    message.success('已复制到剪贴板')
  }

  return (
    <Modal
      title={`编辑 YAML - ${resourceType}/${resourceName}`}
      open={visible}
      onCancel={onClose}
      width={900}
      footer={
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button icon={<CopyOutlined />} onClick={handleCopy}>
            复制
          </Button>
          <Button icon={<ReloadOutlined />} onClick={fetchYAML}>
            刷新
          </Button>
          <Button type="primary" icon={<SaveOutlined />} onClick={handleSave} loading={saving}>
            保存
          </Button>
        </Space>
      }
    >
      {loading ? (
        <div style={{ textAlign: 'center', padding: 50 }}>
          <Spin size="large" />
        </div>
      ) : (
        <textarea
          value={yaml}
          onChange={(e) => setYaml(e.target.value)}
          style={{
            width: '100%',
            height: 500,
            fontFamily: 'Consolas, Monaco, monospace',
            fontSize: 13,
            padding: 16,
            border: '1px solid #d9d9d9',
            borderRadius: 8,
            background: '#f5f5f5',
            resize: 'vertical',
          }}
          spellCheck={false}
        />
      )}
    </Modal>
  )
}

function getAuthToken(): string {
  const token = localStorage.getItem('auth-storage')
  if (token) {
    try {
      const authData = JSON.parse(token)
      return authData?.state?.token || ''
    } catch { return '' }
  }
  return ''
}

export default YAMLEditor
