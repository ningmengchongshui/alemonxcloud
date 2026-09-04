import type { PropsWithChildren } from 'react'
import classNames from 'classnames'
import type { CurrentUser, Page } from '@/types/cloud'

interface ShellProps {
  user: CurrentUser
  page: Page
  onPageChange: (page: Page) => void
  onLogout: () => void
}

export function Shell({ children, user, page, onPageChange, onLogout }: PropsWithChildren<ShellProps>) {
  const displayName = user.username?.trim() || '未命名用户'
  const nav: Array<[Page, string, string]> = [['instances', '▦', '我的实例'], ['create', '＋', '创建 AlemonX'], ...(user.isAdmin ? [['admin', '◇', '超级管理台'] as [Page, string, string]] : [])]
  return <div className="shell"><aside className="side"><div className="brand"><span className="logo">◢</span>AlemonX <small>CLOUD</small></div><nav>{nav.map(([key, icon, label]) => <button key={key} className={classNames({ active: page === key })} onClick={() => onPageChange(key)}><span className="icon">{icon}</span>{label}</button>)}</nav><div className="side-bottom"><button className="profile" onClick={onLogout}><i>{displayName.slice(0, 2).toUpperCase()}</i><span><b>{displayName}</b><small>{user.isAdmin ? '平台管理员' : '个人账户'}</small></span></button></div></aside><main className="main">{children}</main></div>
}
