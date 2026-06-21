import { useState, useEffect } from 'react'
import {
  Card,
  Button,
  Input,
  Space,
  Typography,
  message,
  Steps,
  Alert,
  List,
  Tag,
  Modal,
  Result,
} from 'antd'
import {
  SafetyOutlined,
  CopyOutlined,
  CheckCircleOutlined,
  LockOutlined,
  MobileOutlined,
} from '@ant-design/icons'
import { get2FAStatus, setup2FA, verifyAndEnable2FA, disable2FA } from '../../api/system'

const { Title, Text } = Typography

const TwoFactorAuth: React.FC = () => {
  const [status, setStatus] = useState<{ configured: boolean; enabled: boolean }>({ configured: false, enabled: false })
  const [loading, setLoading] = useState(false)
  const [step, setStep] = useState(0)
  const [secret, setSecret] = useState('')
  const [qrCodeURL, setQRCodeURL] = useState('')
  const [backupCodes, setBackupCodes] = useState<string[]>([])
  const [verifyCode, setVerifyCode] = useState('')
  const [disableCode, setDisableCode] = useState('')
  const [showDisableModal, setShowDisableModal] = useState(false)

  useEffect(() => {
    fetchStatus()
  }, [])

  const fetchStatus = async () => {
    try {
      const res = await get2FAStatus()
      setStatus(res.data)
    } catch (error) {
      console.error('Failed to fetch 2FA status:', error)
    }
  }

  const handleSetup = async () => {
    setLoading(true)
    try {
      const res = await setup2FA()
      setSecret(res.data.secret)
      setQRCodeURL(res.data.qr_code_url)
      setBackupCodes(res.data.backup_codes)
      setStep(1)
    } catch (error) {
      message.error('初始化两步验证失败')
    } finally {
      setLoading(false)
    }
  }

  const handleVerify = async () => {
    if (!verifyCode || verifyCode.length !== 6) {
      message.warning('请输入6位验证码')
      return
    }
    setLoading(true)
    try {
      await verifyAndEnable2FA(verifyCode)
      message.success('两步验证已启用')
      setStep(3)
      fetchStatus()
    } catch (error) {
      message.error('验证码错误')
    } finally {
      setLoading(false)
    }
  }

  const handleDisable = async () => {
    if (!disableCode) {
      message.warning('请输入验证码')
      return
    }
    setLoading(true)
    try {
      await disable2FA(disableCode)
      message.success('两步验证已禁用')
      setShowDisableModal(false)
      setDisableCode('')
      fetchStatus()
    } catch (error) {
      message.error('验证码错误')
    } finally {
      setLoading(false)
    }
  }

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text)
    message.success('已复制到剪贴板')
  }

  const renderSetup = () => (
    <Steps
      current={step}
      items={[
        { title: '开始设置', description: '点击按钮开始' },
        { title: '扫描二维码', description: '使用认证器扫描' },
        { title: '备份恢复码', description: '保存恢复码' },
        { title: '完成', description: '两步验证已启用' },
      ]}
    />
  )

  return (
    <div>
      <Title level={4}>
        <SafetyOutlined /> 两步验证
      </Title>

      <Card style={{ marginBottom: 24 }}>
        <Space direction="vertical" style={{ width: '100%' }} size="large">
          <Alert
            message="什么是两步验证？"
            description="两步验证为您的账户添加额外的安全层。启用后，登录时除了密码外，还需要输入认证器应用生成的验证码。"
            type="info"
            showIcon
          />

          <div>
            <Text strong>当前状态: </Text>
            {status.enabled ? (
              <Tag color="success" icon={<CheckCircleOutlined />}>已启用</Tag>
            ) : (
              <Tag color="default">未启用</Tag>
            )}
          </div>

          {!status.enabled && step === 0 && (
            <Button
              type="primary"
              icon={<MobileOutlined />}
              onClick={handleSetup}
              loading={loading}
              size="large"
            >
              启用两步验证
            </Button>
          )}

          {step > 0 && renderSetup()}
        </Space>
      </Card>

      {/* 步骤 1: 扫描二维码 */}
      {step === 1 && (
        <Card title="步骤 1: 使用认证器扫描二维码" style={{ marginBottom: 24 }}>
          <Space direction="vertical" style={{ width: '100%', textAlign: 'center' }} size="large">
            <div style={{ padding: 20, background: '#f5f5f5', borderRadius: 8 }}>
              <img
                src={`https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=${encodeURIComponent(qrCodeURL)}`}
                alt="QR Code"
                style={{ width: 200, height: 200 }}
              />
            </div>

            <Text type="secondary">
              使用 Google Authenticator、Microsoft Authenticator 或其他 TOTP 应用扫描
            </Text>

            <div>
              <Text>无法扫描？手动输入密钥: </Text>
              <Text code copyable>{secret}</Text>
            </div>

            <Button type="primary" onClick={() => setStep(2)}>
              下一步
            </Button>
          </Space>
        </Card>
      )}

      {/* 步骤 2: 备份恢复码 */}
      {step === 2 && (
        <Card title="步骤 2: 保存恢复码" style={{ marginBottom: 24 }}>
          <Space direction="vertical" style={{ width: '100%' }} size="large">
            <Alert
              message="请妥善保存恢复码"
              description="如果您无法使用认证器应用，可以使用恢复码登录。每个恢复码只能使用一次。"
              type="warning"
              showIcon
            />

            <div style={{ background: '#f5f5f5', padding: 16, borderRadius: 8 }}>
              <List
                size="small"
                dataSource={backupCodes}
                renderItem={(code) => (
                  <List.Item>
                    <Text code>{code}</Text>
                    <Button
                      type="link"
                      icon={<CopyOutlined />}
                      onClick={() => copyToClipboard(code)}
                    >
                      复制
                    </Button>
                  </List.Item>
                )}
              />
            </div>

            <Button
              icon={<CopyOutlined />}
              onClick={() => copyToClipboard(backupCodes.join('\n'))}
            >
              复制所有恢复码
            </Button>

            <div>
              <Text>输入认证器显示的验证码以确认: </Text>
              <Input
                value={verifyCode}
                onChange={(e) => setVerifyCode(e.target.value)}
                placeholder="6位验证码"
                maxLength={6}
                style={{ width: 200, marginTop: 8 }}
              />
            </div>

            <Button type="primary" onClick={handleVerify} loading={loading}>
              验证并启用
            </Button>
          </Space>
        </Card>
      )}

      {/* 步骤 3: 完成 */}
      {step === 3 && (
        <Card style={{ marginBottom: 24 }}>
          <Result
            status="success"
            title="两步验证已启用"
            subTitle="您的账户现在更加安全了。下次登录时需要输入验证码。"
            extra={[
              <Button type="primary" key="done" onClick={() => setStep(0)}>
                完成
              </Button>,
            ]}
          />
        </Card>
      )}

      {/* 已启用状态 */}
      {status.enabled && step === 0 && (
        <Card>
          <Space direction="vertical" style={{ width: '100%' }} size="large">
            <Result
              status="success"
              title="两步验证已启用"
              subTitle="您的账户受到两步验证保护"
            />

            <Button
              danger
              icon={<LockOutlined />}
              onClick={() => setShowDisableModal(true)}
            >
              禁用两步验证
            </Button>
          </Space>
        </Card>
      )}

      {/* 禁用确认弹窗 */}
      <Modal
        title="禁用两步验证"
        open={showDisableModal}
        onOk={handleDisable}
        onCancel={() => {
          setShowDisableModal(false)
          setDisableCode('')
        }}
        confirmLoading={loading}
        okText="确认禁用"
        okButtonProps={{ danger: true }}
      >
        <Space direction="vertical" style={{ width: '100%' }}>
          <Alert
            message="禁用两步验证将降低账户安全性"
            type="warning"
            showIcon
          />
          <div>
            <Text>请输入认证器验证码以确认禁用: </Text>
            <Input
              value={disableCode}
              onChange={(e) => setDisableCode(e.target.value)}
              placeholder="6位验证码"
              maxLength={6}
              style={{ marginTop: 8 }}
            />
          </div>
        </Space>
      </Modal>
    </div>
  )
}

export default TwoFactorAuth
