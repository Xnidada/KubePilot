import { Alert, Card, Typography } from 'antd'
import { AppstoreOutlined } from '@ant-design/icons'

const { Title, Paragraph, Text } = Typography

/**
 * AppStore UI is intentionally a placeholder until Helm catalog APIs exist.
 * Mock install cards were removed to avoid false success impressions.
 */
const AppStoreList: React.FC = () => {
  return (
    <div>
      <Title level={4} style={{ marginBottom: 16 }}>
        <AppstoreOutlined /> 应用商店
      </Title>
      <Card>
        <Alert
          type="info"
          showIcon
          message="功能尚未实现"
          description={
            <div>
              <Paragraph style={{ marginBottom: 8 }}>
                应用商店目前仅为模块壳（模型/菜单元数据），尚无 Helm Chart 安装、仓库同步等 HTTP API。
              </Paragraph>
              <Text type="secondary">
                可在「系统 → 功能模块」中保持 <Text code>appstore</Text> 关闭；实现后再启用。
              </Text>
            </div>
          }
        />
      </Card>
    </div>
  )
}

export default AppStoreList
