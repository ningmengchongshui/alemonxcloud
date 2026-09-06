import { useEffect, useRef, useState } from 'react'
import { FitAddon } from '@xterm/addon-fit'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { Alert, Button, EmptyState, PageHeader } from '@/components/ui'
import type { Instance } from '@/types/cloud'

function terminalSocketURL(instanceID: string) {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${window.location.host}/api/instances/${encodeURIComponent(instanceID)}/terminal`
}

export function InstanceTerminalPage({ instanceID, instance, onBack }: { instanceID: string; instance?: Instance; onBack: () => void }) {
  const host = useRef<HTMLDivElement>(null)
  const [state, setState] = useState<'connecting' | 'connected' | 'closed'>('connecting')
  const [error, setError] = useState('')
  const ready = instance?.runtimeStatus === 'running'

  useEffect(() => {
    if (!instance || !ready || !host.current) return
    const terminal = new Terminal({ cursorBlink: true, fontSize: 13, fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace', theme: { background: '#020617', foreground: '#e2e8f0', cursor: '#93c5fd' } })
    const fit = new FitAddon()
    terminal.loadAddon(fit)
    terminal.open(host.current)
    fit.fit()
    terminal.writeln('\x1b[36mXCloud 容器终端\x1b[0m')
    const socket = new WebSocket(terminalSocketURL(instanceID))
    socket.onopen = () => { setState('connected'); terminal.writeln('\x1b[90m已建立安全连接。\x1b[0m') }
    socket.onmessage = event => terminal.write(typeof event.data === 'string' ? event.data : '')
    socket.onerror = () => setError('终端连接失败，请确认实例和节点均处于运行状态。')
    socket.onclose = () => { setState('closed'); terminal.writeln('\r\n\x1b[90m终端连接已关闭。\x1b[0m') }
    const input = terminal.onData(data => { if (socket.readyState === WebSocket.OPEN) socket.send(data) })
    const resize = () => fit.fit()
    window.addEventListener('resize', resize)
    return () => { input.dispose(); window.removeEventListener('resize', resize); socket.close(); terminal.dispose() }
  }, [instance, instanceID, ready])

  if (!instance) return <EmptyState title="实例不存在" description={`未找到实例 ${instanceID}。`} action={<Button onClick={onBack}>返回实例</Button>} />
  return <section className="page me-page">
    <PageHeader title="实例终端" description={`${instance.name} · ${instance.image} · ${instance.version}`} actions={<div className="flex items-center gap-3"><span className={`text-[11px] font-bold ${state === 'connected' ? 'text-emerald-700' : 'text-slate-400'}`}>{state === 'connected' ? '● 已连接' : state === 'connecting' ? '○ 正在连接' : '○ 已关闭'}</span><Button tone="secondary" onClick={onBack}>返回实例</Button></div>} />
    {!ready ? <Alert tone="info">终端将在实例运行后可用，请等待部署或启动完成。</Alert> : <><div ref={host} className="h-[calc(100dvh-15rem)] min-h-112 overflow-hidden rounded-xl border border-slate-800 bg-slate-950 p-3 shadow-sm" aria-label="容器终端" />{error && <Alert tone="error">{error}</Alert>}</>}
  </section>
}
