import { useEffect, useRef, useState } from 'react'
import { FitAddon } from '@xterm/addon-fit'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { Alert, Button, EmptyState, FilterTabs, LoadingState, PageHeader } from '@/components/ui'
import type { Instance } from '@/types/cloud'
import { useGetWorkspaceFilesQuery, useLazyGetWorkspaceFileQuery, useSaveWorkspaceFileMutation, useUploadWorkspaceFileMutation } from '@/services/cloudApi'

function terminalSocketURL(instanceID: string) {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${window.location.host}/api/instances/${encodeURIComponent(instanceID)}/terminal`
}

export function InstanceTerminalPage({ instanceID, instance, onBack }: { instanceID: string; instance?: Instance; onBack: () => void }) {
  const host = useRef<HTMLDivElement>(null)
  const [state, setState] = useState<'connecting' | 'connected' | 'closed'>('connecting')
  const [error, setError] = useState('')
  const [mode, setMode] = useState<'terminal' | 'files'>('terminal')
  const [path, setPath] = useState('')
  const [editor, setEditor] = useState<{ path: string; content: string }>()
  const { data: listing, isFetching: filesLoading, refetch } = useGetWorkspaceFilesQuery({ id: instanceID, path }, { skip: mode !== 'files' })
  const [openFile, { isFetching: opening }] = useLazyGetWorkspaceFileQuery()
  const [saveFile, { isLoading: saving }] = useSaveWorkspaceFileMutation()
  const [uploadFile, { isLoading: uploading }] = useUploadWorkspaceFileMutation()
  const ready = instance?.runtimeStatus === 'running'

  useEffect(() => {
  if (!instance || !ready || !host.current || mode !== 'terminal') return
    const terminal = new Terminal({ cursorBlink: true, fontSize: 13, fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace', theme: { background: '#020617', foreground: '#e2e8f0', cursor: '#93c5fd' } })
    const fit = new FitAddon()
    terminal.loadAddon(fit)
    terminal.open(host.current)
    fit.fit()
    const socket = new WebSocket(terminalSocketURL(instanceID))
    socket.onopen = () => setState('connected')
    socket.onmessage = event => terminal.write(typeof event.data === 'string' ? event.data : '')
    socket.onerror = () => setError('终端连接失败，请确认实例和节点均处于运行状态。')
    socket.onclose = () => setState('closed')
    const input = terminal.onData(data => { if (socket.readyState === WebSocket.OPEN) socket.send(data) })
    const resize = () => fit.fit()
    window.addEventListener('resize', resize)
    return () => { input.dispose(); window.removeEventListener('resize', resize); socket.close(); terminal.dispose() }
  }, [instance, instanceID, ready, mode])

  const openEntry = async (entry: { path: string; kind: string }) => {
    if (entry.kind === 'directory') { setPath(entry.path); setEditor(undefined); return }
    if (entry.kind !== 'file') return
    const file = await openFile({ id: instanceID, path: entry.path }).unwrap()
    setEditor({ path: file.path, content: file.content })
  }
  const upload = async (file?: File) => {
    if (!file) return
    const encoded = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onerror = () => reject(new Error('无法读取待上传文件'))
      reader.onload = () => resolve(String(reader.result).split(',', 2)[1] ?? '')
      reader.readAsDataURL(file)
    })
    await uploadFile({ id: instanceID, path: `${path ? `${path}/` : ''}${file.name}`, content: encoded }).unwrap()
    void refetch()
  }

  if (!instance) return <EmptyState title="实例不存在" description={`未找到实例 ${instanceID}。`} action={<Button onClick={onBack}>返回实例</Button>} />
  return <section className="min-h-dvh bg-slate-50 px-4 py-4 dark:bg-slate-950 sm:px-6">
    <PageHeader title="实例终端" description={`${instance.name} · ${instance.image} · ${instance.version}`} actions={<div className="flex items-center gap-3"><span className={`text-[11px] font-bold ${state === 'connected' ? 'text-emerald-700' : 'text-slate-400'}`}>{state === 'connected' ? '● 已连接' : state === 'connecting' ? '○ 正在连接' : '○ 已关闭'}</span><Button tone="secondary" onClick={onBack}>返回实例</Button></div>} />
    <FilterTabs value={mode} onChange={setMode} label="实例工作台模式" items={[{ value: 'terminal', label: '终端' }, { value: 'files', label: '文件' }]} />
    {!ready && mode === 'terminal' ? <Alert tone="info">终端将在实例运行后可用，请等待部署或启动完成。</Alert> : mode === 'terminal' ? <><div ref={host} className="mt-3 h-[calc(100dvh-18rem)] min-h-112 overflow-hidden rounded-xl border border-slate-800 bg-slate-950 p-3 shadow-sm" aria-label="容器终端" />{error && <Alert tone="error">{error}</Alert>}</> : <section className="mt-3 grid min-h-[calc(100dvh-18rem)] grid-cols-[18rem_1fr] overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800 max-[850px]:grid-cols-1"><aside className="border-r border-slate-200 p-3 dark:border-slate-700"><div className="mb-3 flex items-center justify-between gap-2"><p className="m-0 text-xs font-bold">/app/workspace{path ? `/${path}` : ''}</p><label className="cursor-pointer text-[11px] font-bold text-blue-700"><input className="sr-only" type="file" onChange={event => void upload(event.target.files?.[0])} />{uploading ? '上传中…' : '上传'}</label></div>{path && <Button tone="ghost" className="mb-2" onClick={() => setPath(path.split('/').slice(0, -1).join('/'))}>返回上级</Button>}{filesLoading ? <LoadingState>正在读取文件…</LoadingState> : <div className="space-y-1">{listing?.entries.map(entry => <button key={entry.path} type="button" onClick={() => void openEntry(entry)} className="flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-xs hover:bg-blue-50 dark:hover:bg-slate-700"><span aria-hidden="true">{entry.kind === 'directory' ? '📁' : '📄'}</span><span className="min-w-0 truncate">{entry.name}</span></button>)}</div>}</aside><main className="flex min-h-80 flex-col p-3">{editor ? <><div className="mb-2 flex items-center justify-between gap-3"><span className="truncate text-xs font-bold">{editor.path}</span><Button loading={saving} onClick={() => void saveFile({ id: instanceID, ...editor }).unwrap().then(() => void refetch())}>保存</Button></div><textarea aria-label="文件内容" value={editor.content} onChange={event => setEditor({ ...editor, content: event.target.value })} spellCheck={false} className="min-h-80 flex-1 resize-none rounded-lg border border-slate-200 bg-slate-950 p-3 font-mono text-xs leading-6 text-slate-100 outline-none focus:border-blue-500 dark:border-slate-600" /></> : <EmptyState title="打开一个文件" description={opening ? '正在读取文件…' : '选择左侧工作区文件即可查看与编辑。'} />}</main></section>}
  </section>
}
