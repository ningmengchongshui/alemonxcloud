import { Component, type ReactNode } from 'react'

interface Props { children: ReactNode }
interface State { hasError: boolean }

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false }
  static getDerivedStateFromError(): State { return { hasError: true } }
  render() {
    if (this.state.hasError) return <main className="auth"><section className="login"><div className="login-card"><p className="eyebrow">AlemonX Cloud</p><h2>页面暂时无法显示</h2><p className="muted">请刷新页面重试；若问题持续存在，请联系平台管理员。</p><button className="primary full" onClick={() => window.location.reload()}>刷新页面</button></div></section></main>
    return this.props.children
  }
}
