import { useEffect, useRef, useState, type PropsWithChildren } from 'react'
import classNames from 'classnames'
import { BrandLogo } from '@/components/BrandLogo'
import { Button, Dialog } from '@/components/ui'
import { XCoinAmount } from '@/components/XCoinMark'
import { TaskNotification } from '@/components/TaskNotification'
import {
  useGetNotificationsQuery,
  useGetRechargeContactQuery,
  useGetWalletQuery,
  useReadAllNotificationsMutation
} from '@/services/cloudApi'
import { trackConsoleEvent } from '@/services/telemetry'
import type { CurrentUser, Page, SuperPage } from '@/types/cloud'

interface ShellProps {
  user: CurrentUser
  area: 'me' | 'super'
  page?: Page
  onPageChange?: (page: Page) => void
  superPage?: SuperPage
  onSuperPageChange?: (page: SuperPage) => void
  onGoToMe: () => void
  onGoToSuper?: () => void
  onLogout: () => void
}
type NavItem = { key: Page | SuperPage; icon: string; label: string }
type NavGroup = { label?: string; items: NavItem[] }

function maskEmail(value: string) {
  const [local, domain] = value.split('@')
  if (!local || !domain) return value
  return `${local.slice(0, 2)}${'*'.repeat(Math.max(1, local.length - 2))}@${domain}`
}
function maskPhone(value: string) {
  return value.length < 7 ? value : `${value.slice(0, 3)}****${value.slice(-4)}`
}

export function Shell({
  children,
  user,
  area,
  page,
  onPageChange,
  superPage,
  onSuperPageChange,
  onGoToMe,
  onGoToSuper,
  onLogout
}: PropsWithChildren<ShellProps>) {
  const displayName = user.username?.trim() || '未命名用户'
  const [profileOpen, setProfileOpen] = useState(false)
  const [helpOpen, setHelpOpen] = useState(false)
  const [navOpen, setNavOpen] = useState(false)
  const [collapsed, setCollapsed] = useState(false)
  const [rechargeOpen, setRechargeOpen] = useState(false)
  const profileRef = useRef<HTMLDivElement>(null)
  const helpRef = useRef<HTMLDivElement>(null)
  const { data: notifications = [] } = useGetNotificationsQuery(undefined, {
    pollingInterval: 30000
  })
  const [readAll] = useReadAllNotificationsMutation()
  const { data: wallet, isLoading: walletLoading } = useGetWalletQuery()
  const { data: rechargeContact } = useGetRechargeContactQuery()
  const unread = notifications.filter(item => !item.readAt).length
  const activeKey = area === 'me' ? page : superPage
  const userGroups: NavGroup[] = [
    {
      items: [
        { key: 'overview', icon: '◇', label: '控制台总览' },
        { key: 'instances', icon: '▦', label: '我的实例' },
        { key: 'create', icon: '＋', label: '创建服务' },
        { key: 'orders', icon: '□', label: '订单中心' }
      ]
    },
    {
      label: '账户中心',
      items: [
        { key: 'wallet', icon: '◈', label: '钱包与流水' },
        {
          key: 'notifications',
          icon: '●',
          label: unread ? `站内通知（${unread}）` : '站内通知'
        },
        { key: 'tickets', icon: '◉', label: '工单支持' }
      ]
    }
  ]
  const superGroups: NavGroup[] = [
    { items: [{ key: 'overview', icon: '◇', label: '运营总览' }] },
    {
      label: '交付运营',
      items: [
        { key: 'orders', icon: '□', label: '订单记录' },
        { key: 'tasks', icon: '↻', label: '任务队列' },
        { key: 'tickets', icon: '◉', label: '工单管理' }
      ]
    },
    {
      label: '资源供给',
      items: [
        { key: 'catalog', icon: '▤', label: '商品目录' },
        { key: 'images', icon: '◇', label: '镜像来源' },
        { key: 'nodes', icon: '◌', label: '节点管理' }
      ]
    },
    {
      label: '营销运营',
      items: [
        { key: 'benefits', icon: '✦', label: '商业权益方案' },
        { key: 'price-tiers', icon: '≋', label: '套餐阶梯价' },
        { key: 'benefit-redemptions', icon: '◷', label: '权益核销记录' }
      ]
    },
    {
      label: '账户与合规',
      items: [
        { key: 'users', icon: '♙', label: '用户与钱包' },
        { key: 'audit', icon: '◷', label: '安全审计' },
        { key: 'settings', icon: '⚙', label: '平台设置' }
      ]
    }
  ]
  const groups = area === 'me' ? userGroups : superGroups
  const activeLabel =
    groups.flatMap(group => group.items).find(item => item.key === activeKey)
      ?.label ?? '控制台'
  const selectPage = (key: Page | SuperPage) => {
    setNavOpen(false)
    if (area === 'me') onPageChange?.(key as Page)
    else onSuperPageChange?.(key as SuperPage)
  }

  useEffect(() => {
    const close = (event: MouseEvent) => {
      const target = event.target as Node
      if (!profileRef.current?.contains(target)) setProfileOpen(false)
      if (!helpRef.current?.contains(target)) setHelpOpen(false)
    }
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setProfileOpen(false)
        setHelpOpen(false)
        setNavOpen(false)
      }
    }
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', escape)
    return () => {
      document.removeEventListener('mousedown', close)
      document.removeEventListener('keydown', escape)
    }
  }, [])
  useEffect(() => {
    if (activeKey)
      trackConsoleEvent('page_view', area, activeKey, { result: 'success' })
  }, [activeKey, area])

  return (
    <div className="flex min-h-screen bg-slate-50 text-slate-800 dark:bg-slate-900 dark:text-slate-100">
      <TaskNotification />
      {navOpen && (
        <button
          type="button"
          className="fixed inset-0 z-30 bg-slate-950/40 lg:hidden"
          aria-label="关闭导航菜单"
          onClick={() => setNavOpen(false)}
        />
      )}
      <aside
        className={classNames(
          'fixed inset-y-0 left-0 z-40 flex w-64 flex-col border-r border-slate-200 bg-white px-3 py-4 transition-transform max-lg:-translate-x-full dark:border-slate-700 dark:bg-slate-950 lg:sticky lg:top-0 lg:h-screen lg:translate-x-0',
          navOpen && 'max-lg:translate-x-0 max-lg:shadow-2xl',
          collapsed && 'lg:w-16 lg:px-2'
        )}
      >
        <div className="mb-6 flex h-8 items-center justify-between px-2">
          <BrandLogo />
          <button
            type="button"
            className="hidden size-8 place-items-center rounded-md text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800 lg:grid"
            aria-label={collapsed ? '展开导航' : '收起导航'}
            onClick={() => setCollapsed(value => !value)}
          >
            {collapsed ? '›' : '‹'}
          </button>
          <button
            type="button"
            className="grid size-8 place-items-center rounded-md text-slate-500 hover:bg-slate-100 lg:hidden"
            aria-label="关闭导航"
            onClick={() => setNavOpen(false)}
          >
            ×
          </button>
        </div>
        <nav
          className="min-h-0 flex-1 overflow-y-auto"
          aria-label={area === 'me' ? '用户控制台导航' : '超级管理台导航'}
        >
          {groups.map((group, index) => (
            <div className="mb-4" key={group.label ?? index}>
              {group.label && (
                <p
                  className={classNames(
                    'mb-1 px-3 text-[10px] font-extrabold tracking-widest text-slate-400',
                    collapsed && 'lg:sr-only'
                  )}
                >
                  {group.label}
                </p>
              )}
              <div className="grid gap-1">
                {group.items.map(item => (
                  <button
                    key={item.key}
                    type="button"
                    title={item.label}
                    aria-current={activeKey === item.key ? 'page' : undefined}
                    onClick={() => selectPage(item.key)}
                    className={classNames(
                      'flex min-h-10 items-center gap-3 rounded-lg px-3 text-left text-xs font-bold transition-colors focus-visible:outline-3 focus-visible:outline-offset-2 focus-visible:outline-blue-200',
                      activeKey === item.key
                        ? 'bg-blue-50 text-blue-700 dark:bg-blue-950 dark:text-blue-200'
                        : 'text-slate-500 hover:bg-slate-100 hover:text-blue-700 dark:text-slate-300 dark:hover:bg-slate-800 dark:hover:text-blue-200',
                      collapsed && 'lg:justify-center lg:px-0'
                    )}
                  >
                    <span
                      className="grid size-6 shrink-0 place-items-center text-base"
                      aria-hidden="true"
                    >
                      {item.icon}
                    </span>
                    <span
                      className={classNames(
                        'truncate',
                        collapsed && 'lg:hidden'
                      )}
                    >
                      {item.label}
                    </span>
                  </button>
                ))}
              </div>
            </div>
          ))}
        </nav>
        <div className="mt-auto space-y-2 border-t border-slate-100 pt-3 dark:border-slate-800">
          {area === 'me' && onGoToSuper && (
            <button
              type="button"
              title="进入超级管理台"
              onClick={onGoToSuper}
              className={classNames(
                'flex min-h-10 w-full items-center gap-3 rounded-lg border border-dashed border-slate-200 px-3 text-left text-[11px] font-bold text-slate-500 hover:border-blue-300 hover:text-blue-700 dark:border-slate-700 dark:text-slate-300',
                collapsed && 'lg:justify-center lg:px-0'
              )}
            >
              <span aria-hidden="true">◇</span>
              <span className={collapsed ? 'lg:hidden' : ''}>
                进入超级管理台
              </span>
            </button>
          )}
          {area === 'super' && (
            <button
              type="button"
              title="返回用户控制台"
              onClick={onGoToMe}
              className={classNames(
                'flex min-h-10 w-full items-center gap-3 rounded-lg border border-dashed border-slate-200 px-3 text-left text-[11px] font-bold text-slate-500 hover:border-blue-300 hover:text-blue-700 dark:border-slate-700 dark:text-slate-300',
                collapsed && 'lg:justify-center lg:px-0'
              )}
            >
              <span aria-hidden="true">←</span>
              <span className={collapsed ? 'lg:hidden' : ''}>
                返回用户控制台
              </span>
            </button>
          )}
          <div ref={profileRef} className="relative">
            <button
              type="button"
              onClick={() => {
                setProfileOpen(value => !value)
                setHelpOpen(false)
              }}
              aria-expanded={profileOpen}
              className={classNames(
                'flex min-h-11 w-full items-center gap-2 rounded-lg bg-slate-100 px-3 text-left hover:bg-slate-200 dark:bg-slate-900 dark:hover:bg-slate-800',
                collapsed && 'lg:justify-center lg:px-0'
              )}
            >
              {user.avatar ? (
                <img
                  src={user.avatar}
                  alt=""
                  className="size-7 shrink-0 rounded-full object-cover"
                />
              ) : (
                <span className="grid size-7 shrink-0 place-items-center rounded-full bg-blue-600 text-[10px] font-extrabold text-white">
                  {displayName.slice(0, 2).toUpperCase()}
                </span>
              )}
              <span
                className={classNames(
                  'min-w-0 flex-1',
                  collapsed && 'lg:hidden'
                )}
              >
                <b className="block truncate text-xs">{displayName}</b>
                <small className="mt-0.5 block truncate text-[9px] text-slate-400">
                  {area === 'super' ? '超级管理员' : '个人工作区'}
                </small>
              </span>
            </button>
            {profileOpen && (
              <div className="absolute bottom-[calc(100%+8px)] left-0 z-50 w-64 rounded-xl border border-slate-200 bg-white p-4 shadow-xl dark:border-slate-700 dark:bg-slate-800">
                <div className="flex items-center gap-2.5">
                  {user.avatar ? (
                    <img
                      src={user.avatar}
                      alt=""
                      className="size-9 rounded-full object-cover"
                    />
                  ) : (
                    <span className="grid size-9 place-items-center rounded-full bg-blue-600 text-xs font-extrabold text-white">
                      {displayName.slice(0, 2).toUpperCase()}
                    </span>
                  )}
                  <div className="min-w-0">
                    <b className="block truncate text-sm">{displayName}</b>
                    <small className="block truncate text-[10px] text-slate-400">
                      ID · {user.id || '—'}
                    </small>
                  </div>
                </div>
                <dl className="my-4 space-y-2 border-y border-slate-100 py-3 text-[11px] dark:border-slate-700">
                  {user.phone && (
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-slate-400">手机号</dt>
                      <dd className="m-0 font-bold text-slate-700 dark:text-slate-100">
                        {maskPhone(user.phone)}
                      </dd>
                    </div>
                  )}
                  {user.email && (
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-slate-400">邮箱</dt>
                      <dd className="m-0 max-w-36 truncate font-bold text-slate-700 dark:text-slate-100">
                        {maskEmail(user.email)}
                      </dd>
                    </div>
                  )}
                  <div className="flex items-center justify-between gap-3">
                    <dt className="text-slate-400">可用余额</dt>
                    <dd className="m-0 inline-flex items-center gap-1 font-bold text-slate-700 dark:text-slate-100">
                      {walletLoading ? (
                        '同步中'
                      ) : (
                        <XCoinAmount
                          value={((wallet?.balanceFen ?? 0) / 100).toFixed(2)}
                        />
                      )}
                    </dd>
                  </div>
                </dl>
                <div className="grid grid-cols-2 gap-1">
                  <button
                    type="button"
                    onClick={() => setRechargeOpen(true)}
                    className="flex min-h-10 items-center justify-between rounded-md px-3 text-xs font-bold text-blue-700 hover:bg-blue-50 dark:text-blue-200 dark:hover:bg-blue-950"
                  >
                    充值 <span aria-hidden="true">↗</span>
                  </button>
                  <a
                    href="https://auth.alemonjs.com"
                    target="_blank"
                    rel="noreferrer"
                    className="flex min-h-10 items-center justify-between rounded-md px-3 text-xs font-bold text-blue-700 hover:bg-blue-50 dark:text-blue-200 dark:hover:bg-blue-950"
                  >
                    安全中心 <span aria-hidden="true">↗</span>
                  </a>
                </div>
                <Button
                  tone="danger"
                  className="mt-2 w-full"
                  onClick={onLogout}
                >
                  退出登录
                </Button>
              </div>
            )}
          </div>
        </div>
      </aside>
      {rechargeOpen && (
        <Dialog
          title="人工充值"
          description="当前仅限人工充值，请点击加入售前咨询群联系官方人员。"
          onClose={() => setRechargeOpen(false)}
        >
          <div className="space-y-4">
            {rechargeContact?.url ? (
              <a
                href={rechargeContact.url}
                target="_blank"
                rel="noreferrer"
                className="flex min-h-11 items-center justify-between rounded-md border border-blue-200 bg-blue-50 px-3 text-xs font-bold text-blue-700 hover:bg-blue-100 dark:border-blue-900 dark:bg-blue-950 dark:text-blue-200"
              >
                {rechargeContact.name}
                <span aria-hidden="true">↗</span>
              </a>
            ) : (
              <p className="rounded-lg bg-amber-50 p-3 text-xs leading-5 text-amber-800 dark:bg-amber-950 dark:text-amber-100">
                售前咨询群暂未配置，请联系平台管理员。
              </p>
            )}
            <div className="flex justify-end">
              <Button tone="secondary" onClick={() => setRechargeOpen(false)}>
                关闭
              </Button>
            </div>
          </div>
        </Dialog>
      )}
      <main className="min-w-0 flex-1">
        <header className="sticky top-0 z-20 flex h-13 items-center gap-3 border-b border-slate-200 bg-white/95 px-4 backdrop-blur dark:border-slate-700 dark:bg-slate-950/95 sm:px-7">
          <button
            type="button"
            className="grid size-9 place-items-center rounded-md border border-slate-200 text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-200 lg:hidden"
            aria-label="打开导航"
            onClick={() => setNavOpen(true)}
          >
            ☰
          </button>
          <div className="min-w-0 text-xs">
            <span className="font-bold text-slate-700 dark:text-slate-100">
              {area === 'super' ? '超级管理台' : '用户控制台'}
            </span>
            <span className="mx-2 text-slate-300">/</span>
            <b className="text-blue-700 dark:text-blue-200">{activeLabel}</b>
          </div>
          <div ref={helpRef} className="relative ml-auto">
            <button
              type="button"
              className="rounded-md px-2 py-1.5 text-[11px] font-bold text-slate-500 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800"
              onClick={() => {
                setHelpOpen(value => !value)
                setProfileOpen(false)
              }}
            >
              帮助
            </button>
            {helpOpen && (
              <div className="absolute right-0 top-[calc(100%+8px)] z-40 w-64 rounded-xl border border-slate-200 bg-white p-4 shadow-xl dark:border-slate-700 dark:bg-slate-800">
                <b className="text-xs">快速上手</b>
                <ol className="mt-2 list-decimal space-y-1 pl-4 text-[11px] leading-5 text-slate-500 dark:text-slate-300">
                  <li>在“创建服务”选择可信镜像与套餐。</li>
                  <li>确认钱包余额与订单摘要。</li>
                  <li>在订单、实例和通知页跟踪服务变化。</li>
                </ol>
              </div>
            )}
          </div>
        </header>
        {unread > 0 && (
          <div className="flex items-center justify-between gap-3 border-b border-blue-100 bg-blue-50 px-4 py-2.5 text-[11px] text-blue-800 dark:border-blue-900 dark:bg-blue-950 dark:text-blue-100 sm:px-7">
            <span>你有 {unread} 条未读站内通知</span>
            <button
              type="button"
              className="font-bold underline"
              onClick={() => {
                if (area === 'me') selectPage('notifications')
                else void readAll()
              }}
            >
              查看通知
            </button>
          </div>
        )}
        {children}
      </main>
    </div>
  )
}
