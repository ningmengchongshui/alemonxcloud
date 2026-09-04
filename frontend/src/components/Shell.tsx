import { useEffect, useRef, useState, type PropsWithChildren } from 'react'
import classNames from 'classnames'
import { BrandLogo } from '@/components/BrandLogo'
import { useGetNotificationsQuery, useReadAllNotificationsMutation } from '@/services/cloudApi'
import type { CurrentUser, Page, SuperPage } from '@/types/cloud'

interface ShellProps { user: CurrentUser; area: 'me' | 'super'; page?: Page; onPageChange?: (page: Page) => void; superPage?: SuperPage; onSuperPageChange?: (page: SuperPage) => void; onGoToMe: () => void; onGoToSuper?: () => void; onLogout: () => void }

export function Shell({ children, user, area, page, onPageChange, superPage, onSuperPageChange, onGoToMe, onGoToSuper, onLogout }: PropsWithChildren<ShellProps>) {
  const displayName = user.username?.trim() || '未命名用户'
  const [profileOpen, setProfileOpen] = useState(false)
  const profileRef = useRef<HTMLDivElement>(null)
  const { data: notifications = [] } = useGetNotificationsQuery(undefined, { pollingInterval: 30000 })
  const [readAll] = useReadAllNotificationsMutation()
  const unread = notifications.filter(item => !item.readAt).length
  useEffect(() => { const close = (event: MouseEvent) => { if (profileRef.current && !profileRef.current.contains(event.target as Node)) setProfileOpen(false) }; const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') setProfileOpen(false) }; document.addEventListener('mousedown', close);document.addEventListener('keydown', escape);return () => { document.removeEventListener('mousedown', close);document.removeEventListener('keydown', escape) } }, [])
  const userNav: Array<[Page,string,string]> = [['instances','▦','我的实例'],['create','＋','创建'],['orders','□','订单中心']]
  const superNav: Array<[SuperPage,string,string]> = [['overview','◇','总览'],['orders','□','订单处理'],['tasks','↻','任务队列'],['catalog','▤','商品目录'],['nodes','◌','节点与配额'],['users','♙','用户与钱包'],['audit','◷','安全审计']]
  const nav = area === 'me' ? userNav : superNav
  return <div className={`shell shell-${area}`}><aside className="side"><div className="brand"><BrandLogo /></div><nav aria-label={area === 'me' ? '用户控制台导航' : '超级管理台导航'}>{nav.map(([key,icon,label])=><button key={key} className={classNames({active:(area==='me'?page:superPage)===key})} aria-current={(area==='me'?page:superPage)===key?'page':undefined} onClick={()=>area==='me'?onPageChange?.(key as Page):onSuperPageChange?.(key as SuperPage)}><span className="icon">{icon}</span><span className="nav-label">{label}</span></button>)}</nav><div className="side-bottom">{area==='me'&&onGoToSuper&&<button className="workspace-switch" onClick={onGoToSuper}><span className="icon">◇</span><span className="nav-label">进入超级管理台</span></button>}{area==='super'&&<button className="workspace-switch" onClick={onGoToMe}><span className="icon">←</span><span className="nav-label">返回用户控制台</span></button>}<div className="profile-menu" ref={profileRef}>{profileOpen&&<section className="profile-card" id="profile-card" aria-label="账户信息"><div className="profile-card-user"><i>{displayName.slice(0,2).toUpperCase()}</i><div><b>{displayName}</b><span>{area==='super'?'超级管理员':'个人账户'}</span></div></div><p>{area==='super'?'你正在管理平台运营与交付。':'在这里管理你的服务和订阅。'}</p><button className="profile-logout" onClick={onLogout}><span>↗</span>退出登录</button></section>}<button className="profile" onClick={()=>setProfileOpen(value=>!value)} aria-expanded={profileOpen} aria-controls="profile-card"><i>{displayName.slice(0,2).toUpperCase()}</i><span><b>{displayName}</b><small>{area==='super'?'超级管理员':'个人账户'}</small></span><span className="profile-chevron" aria-hidden="true">{profileOpen?'⌃':'⌄'}</span></button></div></div></aside><main className="main">{unread>0&&<div className="notice-bar" role="status"><span>你有 {unread} 条未读站内通知</span><button onClick={()=>void readAll()}>全部标为已读</button></div>}{children}</main></div>
}
