import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Form, Input, Button, Card, message, Typography, Space, Divider } from 'antd'
import { UserOutlined, LockOutlined, SafetyOutlined, GithubOutlined, GoogleOutlined } from '@ant-design/icons'
import { login, verify2FALogin, listPublicOAuthProviders, initiateOAuthLogin, getProfile } from '../api/auth'
import { useAuthStore } from '../stores/auth'

const { Title, Text } = Typography

interface LoginForm {
  username: string
  password: string
}

interface TwoFactorForm {
  code: string
}

const Login: React.FC = () => {
  const [loading, setLoading] = useState(false)
  const [pendingToken, setPendingToken] = useState<string | null>(null)
  const [oauthProviders, setOauthProviders] = useState<Array<{ provider: string; name: string }>>([])
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const { setToken, setUser } = useAuthStore()
  const [loginForm] = Form.useForm<LoginForm>()
  const [twoFactorForm] = Form.useForm<TwoFactorForm>()

  const completeLogin = (data: { token: string; user: any }) => {
    setToken(data.token)
    setUser(data.user)
    message.success('登录成功')
    navigate('/')
  }

  useEffect(() => {
    listPublicOAuthProviders()
      .then((res) => setOauthProviders(res.data || []))
      .catch(() => setOauthProviders([]))
  }, [])

  useEffect(() => {
    const oauthToken = searchParams.get('oauth_token')
    if (!oauthToken) return
    ;(async () => {
      setLoading(true)
      try {
        setToken(oauthToken)
        const profile = await getProfile()
        setUser(profile.data as any)
        message.success('OAuth 登录成功')
        setSearchParams({})
        navigate('/')
      } catch (e) {
        message.error('OAuth 登录失败')
        useAuthStore.getState().logout()
      } finally {
        setLoading(false)
      }
    })()
  }, [searchParams, setToken, setUser, setSearchParams, navigate])

  const onFinish = async (values: LoginForm) => {
    setLoading(true)
    try {
      const res = await login(values)
      if (res.data?.require_2fa) {
        if (!res.data.pending_token) {
          message.error('未获取到两步验证会话，请重试')
          return
        }
        setPendingToken(res.data.pending_token)
        message.info('请输入两步验证码')
        return
      }
      if (!res.data?.token || !res.data?.user) {
        message.error('登录响应无效')
        return
      }
      completeLogin({ token: res.data.token, user: res.data.user })
    } catch (error) {
      console.error('Login failed:', error)
    } finally {
      setLoading(false)
    }
  }

  const onVerify2FA = async (values: TwoFactorForm) => {
    if (!pendingToken) return
    setLoading(true)
    try {
      const res = await verify2FALogin(pendingToken, values.code.trim())
      if (!res.data?.token) {
        message.error('验证成功但未返回令牌')
        return
      }
      completeLogin(res.data)
    } catch (error) {
      console.error('2FA verify failed:', error)
      setPendingToken(null)
      twoFactorForm.resetFields()
    } finally {
      setLoading(false)
    }
  }

  const backToLogin = () => {
    setPendingToken(null)
    twoFactorForm.resetFields()
  }

  const onOAuth = async (provider: string) => {
    setLoading(true)
    try {
      const res = await initiateOAuthLogin(provider)
      const url = res.data?.auth_url
      if (!url) {
        message.error('未获取到授权地址')
        return
      }
      window.location.href = url
    } catch (e) {
      console.error(e)
    } finally {
      setLoading(false)
    }
  }

  const oauthIcon = (provider: string) => {
    if (provider === 'github') return <GithubOutlined />
    if (provider === 'google') return <GoogleOutlined />
    return <SafetyOutlined />
  }

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
      }}
    >
      <Card
        style={{
          width: 400,
          boxShadow: '0 20px 60px rgba(0,0,0,0.3)',
          borderRadius: 12,
        }}
      >
        <div style={{ textAlign: 'center', marginBottom: 32 }}>
          <Title level={2} style={{ marginBottom: 8 }}>
            🚀 KubePilot
          </Title>
          <Text type="secondary">
            {pendingToken ? '两步验证' : 'K8S 智能运维管理平台'}
          </Text>
        </div>

        {pendingToken ? (
          <Form
            form={twoFactorForm}
            name="twofactor"
            onFinish={onVerify2FA}
            size="large"
            autoComplete="off"
          >
            <Form.Item
              name="code"
              rules={[{ required: true, message: '请输入验证码或备份码' }]}
            >
              <Input
                prefix={<SafetyOutlined />}
                placeholder="6 位验证码或备份码"
                maxLength={16}
                autoFocus
              />
            </Form.Item>
            <Form.Item>
              <Button type="primary" htmlType="submit" loading={loading} block style={{ height: 44 }}>
                验证并登录
              </Button>
            </Form.Item>
            <Space style={{ width: '100%', justifyContent: 'center' }}>
              <Button type="link" onClick={backToLogin}>
                返回重新登录
              </Button>
            </Space>
          </Form>
        ) : (
          <>
            <Form form={loginForm} name="login" onFinish={onFinish} size="large" autoComplete="off">
              <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
                <Input prefix={<UserOutlined />} placeholder="用户名" autoComplete="off" />
              </Form.Item>
              <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
                <Input.Password prefix={<LockOutlined />} placeholder="密码" autoComplete="new-password" />
              </Form.Item>
              <Form.Item>
                <Button type="primary" htmlType="submit" loading={loading} block style={{ height: 44 }}>
                  登录
                </Button>
              </Form.Item>
            </Form>
            {oauthProviders.length > 0 && (
              <>
                <Divider plain>或使用 SSO</Divider>
                <Space direction="vertical" style={{ width: '100%' }}>
                  {oauthProviders.map((p) => (
                    <Button
                      key={p.provider}
                      block
                      icon={oauthIcon(p.provider)}
                      loading={loading}
                      onClick={() => onOAuth(p.provider)}
                    >
                      使用 {p.name || p.provider} 登录
                    </Button>
                  ))}
                </Space>
              </>
            )}
          </>
        )}
      </Card>
    </div>
  )
}

export default Login
