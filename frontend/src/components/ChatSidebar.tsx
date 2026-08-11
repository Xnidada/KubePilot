import { useMemo, useState } from 'react'
import { Button, Input, List, Popconfirm, Typography, Space, Tooltip, Empty, Checkbox } from 'antd'
import {
  PlusOutlined,
  DeleteOutlined,
  EditOutlined,
  CheckOutlined,
  CloseOutlined,
  MessageOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  ClearOutlined,
  CheckSquareOutlined,
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
  onDelete: (id: string) => void | Promise<void>
  onBatchDelete?: (ids: string[]) => void | Promise<void>
  onRename: (id: string, title: string) => void | Promise<void>
  onClear?: (id: string) => void | Promise<void>
  collapsed?: boolean
  onToggleCollapse?: () => void
  deletingId?: string | null
  batchDeleting?: boolean
}

const ChatSidebar: React.FC<ChatSidebarProps> = ({
  conversations,
  activeId,
  onSelect,
  onCreate,
  onDelete,
  onBatchDelete,
  onRename,
  onClear,
  collapsed = false,
  onToggleCollapse,
  deletingId = null,
  batchDeleting = false,
}) => {
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editValue, setEditValue] = useState('')
  const [hoveredId, setHoveredId] = useState<string | null>(null)
  const [selectMode, setSelectMode] = useState(false)
  const [selectedIds, setSelectedIds] = useState<string[]>([])

  const allIds = useMemo(() => conversations.map(c => c.id), [conversations])
  const allSelected = allIds.length > 0 && selectedIds.length === allIds.length
  const partiallySelected = selectedIds.length > 0 && !allSelected

  const exitSelectMode = () => {
    setSelectMode(false)
    setSelectedIds([])
  }

  const enterSelectMode = () => {
    setEditingId(null)
    setSelectMode(true)
    setSelectedIds([])
  }

  const toggleSelect = (id: string) => {
    setSelectedIds(prev => (prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id]))
  }

  const toggleSelectAll = () => {
    setSelectedIds(allSelected ? [] : allIds)
  }

  const handleStartEdit = (id: string, currentTitle: string) => {
    setEditingId(id)
    setEditValue(currentTitle)
  }

  const handleSaveEdit = async () => {
    if (editingId && editValue.trim()) {
      await onRename(editingId, editValue.trim())
    }
    setEditingId(null)
  }

  const handleCancelEdit = () => {
    setEditingId(null)
  }

  const handleBatchDelete = async () => {
    if (selectedIds.length === 0 || !onBatchDelete) return
    await onBatchDelete(selectedIds)
    exitSelectMode()
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
              type={activeId === item.id ? 'primary' : 'text'}
              icon={<MessageOutlined />}
              onClick={() => onSelect(item.id)}
              style={{ width: 36, height: 36 }}
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
        {selectMode ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <Checkbox
                checked={allSelected}
                indeterminate={partiallySelected}
                onChange={toggleSelectAll}
                disabled={conversations.length === 0 || batchDeleting}
              >
                全选
              </Checkbox>
              <Text type="secondary" style={{ fontSize: 12 }}>
                已选 {selectedIds.length}/{conversations.length}
              </Text>
            </div>
            <Space style={{ width: '100%' }}>
              <Popconfirm
                title={`删除选中的 ${selectedIds.length} 个会话？`}
                description="会话及其全部消息将永久删除。"
                okText="删除"
                okButtonProps={{ danger: true, loading: batchDeleting }}
                cancelText="取消"
                disabled={selectedIds.length === 0 || batchDeleting}
                onConfirm={() => void handleBatchDelete()}
              >
                <Button
                  danger
                  icon={<DeleteOutlined />}
                  disabled={selectedIds.length === 0}
                  loading={batchDeleting}
                  style={{ flex: 1 }}
                >
                  删除所选
                </Button>
              </Popconfirm>
              <Button onClick={exitSelectMode} disabled={batchDeleting}>
                取消
              </Button>
            </Space>
          </div>
        ) : (
          <div style={{ display: 'flex', gap: 8 }}>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={onCreate}
              style={{ borderRadius: 8, flex: 1 }}
            >
              新建对话
            </Button>
            {onBatchDelete && conversations.length > 0 && (
              <Tooltip title="多选删除">
                <Button icon={<CheckSquareOutlined />} onClick={enterSelectMode} />
              </Tooltip>
            )}
            {onToggleCollapse && (
              <Tooltip title="收起侧边栏">
                <Button icon={<MenuFoldOutlined />} onClick={onToggleCollapse} />
              </Tooltip>
            )}
          </div>
        )}
      </div>

      <div style={{ flex: 1, overflow: 'auto' }}>
        {conversations.length === 0 ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description="暂无会话"
            style={{ marginTop: 48 }}
          />
        ) : (
          <List
            dataSource={conversations}
            renderItem={(item) => {
              const isActive = activeId === item.id
              const showActions = !selectMode && (isActive || hoveredId === item.id || editingId === item.id)
              const isDeleting = deletingId === item.id || (batchDeleting && selectedIds.includes(item.id))
              const checked = selectedIds.includes(item.id)

              return (
                <div
                  onClick={() => {
                    if (editingId === item.id || isDeleting) return
                    if (selectMode) {
                      toggleSelect(item.id)
                      return
                    }
                    onSelect(item.id)
                  }}
                  onMouseEnter={() => setHoveredId(item.id)}
                  onMouseLeave={() => setHoveredId(null)}
                  style={{
                    padding: '12px 16px',
                    cursor: 'pointer',
                    background: selectMode
                      ? (checked ? '#fff1f0' : hoveredId === item.id ? '#f0f0f0' : 'transparent')
                      : (isActive ? '#e6f4ff' : hoveredId === item.id ? '#f0f0f0' : 'transparent'),
                    borderLeft: selectMode
                      ? (checked ? '3px solid #ff4d4f' : '3px solid transparent')
                      : (isActive ? '3px solid #1890ff' : '3px solid transparent'),
                    transition: 'all 0.2s',
                    opacity: isDeleting ? 0.55 : 1,
                  }}
                >
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 8 }}>
                    {selectMode && (
                      <Checkbox
                        checked={checked}
                        disabled={batchDeleting}
                        onClick={(e) => e.stopPropagation()}
                        onChange={() => toggleSelect(item.id)}
                        style={{ marginTop: 2 }}
                      />
                    )}
                    <div style={{ flex: 1, minWidth: 0, overflow: 'hidden' }}>
                      {editingId === item.id ? (
                        <Space size={4} onClick={(e) => e.stopPropagation()}>
                          <Input
                            size="small"
                            value={editValue}
                            onChange={(e) => setEditValue(e.target.value)}
                            onPressEnter={() => void handleSaveEdit()}
                            autoFocus
                            style={{ width: 150 }}
                          />
                          <Button type="text" size="small" icon={<CheckOutlined />} onClick={() => void handleSaveEdit()} />
                          <Button type="text" size="small" icon={<CloseOutlined />} onClick={handleCancelEdit} />
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
                              {item.title || '新对话'}
                            </Text>
                          </div>
                          <div style={{ marginTop: 4, display: 'flex', justifyContent: 'space-between', gap: 8 }}>
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

                    {!selectMode && editingId !== item.id && (
                      <Space
                        size={0}
                        style={{
                          opacity: showActions ? 1 : 0,
                          pointerEvents: showActions ? 'auto' : 'none',
                          flexShrink: 0,
                          transition: 'opacity 0.15s',
                        }}
                        onClick={(e) => e.stopPropagation()}
                      >
                        <Tooltip title="重命名">
                          <Button
                            type="text"
                            size="small"
                            icon={<EditOutlined />}
                            onClick={() => handleStartEdit(item.id, item.title)}
                          />
                        </Tooltip>
                        {onClear && item.messageCount > 0 && (
                          <Popconfirm
                            title="清空此会话消息？"
                            description="仅清空消息，会话本身会保留。"
                            okText="清空"
                            cancelText="取消"
                            placement="right"
                            onConfirm={() => onClear(item.id)}
                          >
                            <Tooltip title="清空消息">
                              <Button type="text" size="small" icon={<ClearOutlined />} />
                            </Tooltip>
                          </Popconfirm>
                        )}
                        <Popconfirm
                          title="删除此会话？"
                          description="会话及其全部消息将永久删除。"
                          okText="删除"
                          okButtonProps={{ danger: true, loading: isDeleting }}
                          cancelText="取消"
                          placement="right"
                          onConfirm={() => onDelete(item.id)}
                        >
                          <Tooltip title="删除会话">
                            <Button
                              type="text"
                              size="small"
                              danger
                              loading={isDeleting}
                              icon={<DeleteOutlined />}
                            />
                          </Tooltip>
                        </Popconfirm>
                      </Space>
                    )}
                  </div>
                </div>
              )
            }}
          />
        )}
      </div>
    </div>
  )
}

export default ChatSidebar
