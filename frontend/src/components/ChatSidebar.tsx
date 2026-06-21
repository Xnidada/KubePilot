import { useState } from 'react'
import { Button, Input, List, Popconfirm, Typography, Space, Tooltip } from 'antd'
import {
  PlusOutlined,
  DeleteOutlined,
  EditOutlined,
  CheckOutlined,
  CloseOutlined,
  MessageOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
} from '@ant-design/icons'

const { Text } = Typography

export interface Conversation {
  id: string
  title: string
  createdAt: Date
  updatedAt: Date
  messageCount: number
}

interface ChatSidebarProps {
  conversations: Conversation[]
  activeId: string | null
  onSelect: (id: string) => void
  onCreate: () => void
  onDelete: (id: string) => void
  onRename: (id: string, title: string) => void
  collapsed?: boolean
  onToggleCollapse?: () => void
}

const ChatSidebar: React.FC<ChatSidebarProps> = ({
  conversations,
  activeId,
  onSelect,
  onCreate,
  onDelete,
  onRename,
  collapsed = false,
  onToggleCollapse,
}) => {
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editValue, setEditValue] = useState('')

  const handleStartEdit = (id: string, currentTitle: string) => {
    setEditingId(id)
    setEditValue(currentTitle)
  }

  const handleSaveEdit = () => {
    if (editingId && editValue.trim()) {
      onRename(editingId, editValue.trim())
    }
    setEditingId(null)
  }

  const handleCancelEdit = () => {
    setEditingId(null)
  }

  const formatTime = (date: Date) => {
    const now = new Date()
    const diff = now.getTime() - date.getTime()
    const minutes = Math.floor(diff / 60000)
    const hours = Math.floor(diff / 3600000)
    const days = Math.floor(diff / 86400000)

    if (minutes < 1) return '刚刚'
    if (minutes < 60) return `${minutes}分钟前`
    if (hours < 24) return `${hours}小时前`
    if (days < 7) return `${days}天前`
    return date.toLocaleDateString()
  }

  if (collapsed) {
    return (
      <div
        style={{
          width: 48,
          minWidth: 48,
          background: '#f7f7f8',
          borderRight: '1px solid #e5e5e5',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          padding: '16px 0',
          gap: 8,
        }}
      >
        <Tooltip title="展开侧边栏" placement="right">
          <Button
            type="text"
            icon={<MenuUnfoldOutlined />}
            onClick={onToggleCollapse}
            style={{ marginBottom: 8 }}
          />
        </Tooltip>
        <Tooltip title="新建对话" placement="right">
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={onCreate}
            style={{ borderRadius: 8 }}
          />
        </Tooltip>
        {conversations.slice(0, 10).map((item) => (
          <Tooltip key={item.id} title={item.title} placement="right">
            <Button
              type={activeId === item.id ? "primary" : "text"}
              icon={<MessageOutlined />}
              onClick={() => onSelect(item.id)}
              style={{
                width: 36,
                height: 36,
              }}
            />
          </Tooltip>
        ))}
      </div>
    )
  }

  return (
    <div
      style={{
        width: 280,
        minWidth: 280,
        maxWidth: 280,
        background: '#f7f7f8',
        borderRight: '1px solid #e5e5e5',
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        overflow: 'hidden',
      }}
    >
      <div style={{ padding: '12px 16px', borderBottom: '1px solid #e5e5e5', flexShrink: 0 }}>
        <div style={{ display: 'flex', gap: 8 }}>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={onCreate}
            style={{ borderRadius: 8, flex: 1 }}
          >
            新建对话
          </Button>
          {onToggleCollapse && (
            <Tooltip title="收起侧边栏">
              <Button
                icon={<MenuFoldOutlined />}
                onClick={onToggleCollapse}
              />
            </Tooltip>
          )}
        </div>
      </div>

      <div style={{ flex: 1, overflow: 'auto' }}>
        <List
          dataSource={conversations}
          renderItem={(item) => (
            <div
              onClick={() => onSelect(item.id)}
              style={{
                padding: '12px 16px',
                cursor: 'pointer',
                background: activeId === item.id ? '#e6f4ff' : 'transparent',
                borderLeft: activeId === item.id ? '3px solid #1890ff' : '3px solid transparent',
                transition: 'all 0.2s',
              }}
              onMouseEnter={(e) => {
                if (activeId !== item.id) {
                  e.currentTarget.style.background = '#f0f0f0'
                }
              }}
              onMouseLeave={(e) => {
                if (activeId !== item.id) {
                  e.currentTarget.style.background = 'transparent'
                }
              }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 8 }}>
                <div style={{ flex: 1, minWidth: 0, overflow: 'hidden' }}>
                  {editingId === item.id ? (
                    <Space size={4}>
                      <Input
                        size="small"
                        value={editValue}
                        onChange={(e) => setEditValue(e.target.value)}
                        onPressEnter={handleSaveEdit}
                        autoFocus
                        style={{ width: 160 }}
                      />
                      <Button
                        type="text"
                        size="small"
                        icon={<CheckOutlined />}
                        onClick={handleSaveEdit}
                      />
                      <Button
                        type="text"
                        size="small"
                        icon={<CloseOutlined />}
                        onClick={handleCancelEdit}
                      />
                    </Space>
                  ) : (
                    <>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8, overflow: 'hidden' }}>
                        <MessageOutlined style={{ color: '#666', fontSize: 14, flexShrink: 0 }} />
                        <Text
                          strong
                          style={{
                            fontSize: 14,
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                          }}
                        >
                          {item.title}
                        </Text>
                      </div>
                      <div style={{ marginTop: 4, display: 'flex', justifyContent: 'space-between' }}>
                        <Text type="secondary" style={{ fontSize: 12 }}>
                          {item.messageCount} 条消息
                        </Text>
                        <Text type="secondary" style={{ fontSize: 12, flexShrink: 0 }}>
                          {formatTime(item.updatedAt)}
                        </Text>
                      </div>
                    </>
                  )}
                </div>

                {editingId !== item.id && (
                  <Space size={2} style={{ opacity: activeId === item.id ? 1 : 0, flexShrink: 0 }}>
                    <Button
                      type="text"
                      size="small"
                      icon={<EditOutlined />}
                      onClick={(e) => {
                        e.stopPropagation()
                        handleStartEdit(item.id, item.title)
                      }}
                    />
                    <Popconfirm
                      title="确定删除此对话？"
                      onConfirm={(e) => {
                        e?.stopPropagation()
                        onDelete(item.id)
                      }}
                      onCancel={(e) => e?.stopPropagation()}
                    >
                      <Button
                        type="text"
                        size="small"
                        danger
                        icon={<DeleteOutlined />}
                        onClick={(e) => e.stopPropagation()}
                      />
                    </Popconfirm>
                  </Space>
                )}
              </div>
            </div>
          )}
        />
      </div>
    </div>
  )
}

export default ChatSidebar
