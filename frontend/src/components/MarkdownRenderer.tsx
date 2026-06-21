import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter'
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism'
import { CopyOutlined, CheckOutlined } from '@ant-design/icons'
import { Button, message } from 'antd'
import { useState } from 'react'

interface MarkdownRendererProps {
  content: string
}

const MarkdownRenderer: React.FC<MarkdownRendererProps> = ({ content }) => {
  const [copiedCode, setCopiedCode] = useState<string | null>(null)

  const handleCopy = async (code: string) => {
    try {
      await navigator.clipboard.writeText(code)
      setCopiedCode(code)
      message.success('已复制到剪贴板')
      setTimeout(() => setCopiedCode(null), 2000)
    } catch {
      message.error('复制失败')
    }
  }

  if (!content) return null

  return (
    <div style={{ fontSize: 14, lineHeight: 1.6 }}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          code(props) {
            const { children, className, ...rest } = props
            const match = /language-(\w+)/.exec(className || '')
            const codeString = String(children).replace(/\n$/, '')

            if (match) {
              return (
                <div style={{ position: 'relative', margin: '8px 0' }}>
                  <Button
                    type="text"
                    size="small"
                    icon={copiedCode === codeString ? <CheckOutlined /> : <CopyOutlined />}
                    onClick={() => handleCopy(codeString)}
                    style={{
                      position: 'absolute',
                      right: 8,
                      top: 8,
                      color: '#fff',
                      zIndex: 1,
                    }}
                  />
                  <SyntaxHighlighter
                    style={oneDark as any}
                    language={match[1]}
                    PreTag="div"
                    customStyle={{
                      borderRadius: 8,
                      padding: '16px',
                      margin: 0,
                      fontSize: 13,
                    }}
                  >
                    {codeString}
                  </SyntaxHighlighter>
                </div>
              )
            }

            return (
              <code
                className={className}
                style={{
                  background: '#f0f0f0',
                  padding: '2px 6px',
                  borderRadius: 4,
                  fontSize: '0.9em',
                  color: '#e74c3c',
                }}
                {...rest}
              >
                {children}
              </code>
            )
          },
          table({ children }) {
            return (
              <div style={{ overflowX: 'auto', margin: '16px 0' }}>
                <table style={{ borderCollapse: 'collapse', width: '100%' }}>
                  {children}
                </table>
              </div>
            )
          },
          th({ children }) {
            return (
              <th style={{ border: '1px solid #ddd', padding: '8px 12px', background: '#f5f5f5', fontWeight: 600, textAlign: 'left' }}>
                {children}
              </th>
            )
          },
          td({ children }) {
            return (
              <td style={{ border: '1px solid #ddd', padding: '8px 12px' }}>
                {children}
              </td>
            )
          },
          ul({ children }) {
            return <ul style={{ paddingLeft: 24, margin: '8px 0' }}>{children}</ul>
          },
          ol({ children }) {
            return <ol style={{ paddingLeft: 24, margin: '8px 0' }}>{children}</ol>
          },
          li({ children }) {
            return <li style={{ margin: '4px 0' }}>{children}</li>
          },
          h1({ children }) {
            return <h1 style={{ fontSize: '1.5em', fontWeight: 700, margin: '16px 0 8px' }}>{children}</h1>
          },
          h2({ children }) {
            return <h2 style={{ fontSize: '1.3em', fontWeight: 600, margin: '16px 0 8px' }}>{children}</h2>
          },
          h3({ children }) {
            return <h3 style={{ fontSize: '1.1em', fontWeight: 600, margin: '12px 0 8px' }}>{children}</h3>
          },
          p({ children }) {
            return <p style={{ margin: '8px 0', lineHeight: 1.6 }}>{children}</p>
          },
          strong({ children }) {
            return <strong style={{ fontWeight: 600 }}>{children}</strong>
          },
          em({ children }) {
            return <em style={{ fontStyle: 'italic' }}>{children}</em>
          },
          blockquote({ children }) {
            return (
              <blockquote style={{ borderLeft: '4px solid #1890ff', paddingLeft: 16, margin: '16px 0', color: '#666', background: '#f9f9f9', padding: '12px 16px', borderRadius: '0 8px 8px 0' }}>
                {children}
              </blockquote>
            )
          },
          a({ href, children }) {
            return <a href={href} target="_blank" rel="noopener noreferrer" style={{ color: '#1890ff', textDecoration: 'none' }}>{children}</a>
          },
          hr() {
            return <hr style={{ border: 'none', borderTop: '1px solid #eee', margin: '16px 0' }} />
          },
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  )
}

export default MarkdownRenderer
