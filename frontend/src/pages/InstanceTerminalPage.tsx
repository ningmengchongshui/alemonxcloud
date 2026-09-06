import { useEffect, useRef, useState } from 'react'
import { FitAddon } from '@xterm/addon-fit'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { Alert, Button, EmptyState, FilterTabs, LoadingState } from '@/components/ui'
import type { Instance } from '@/types/cloud'
import { useGetWorkspaceFilesQuery, useLazyGetWorkspaceFileQuery, useSaveWorkspaceFileMutation, useUploadWorkspaceFileMutation } from '@/services/cloudApi'

function terminalSocketURL(instanceID: string) {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${window.location.host}/api/instances/${encodeURIComponent(instanceID)}/terminal`
}

export function InstanceTerminalPage({ instanceID, instance, onBack }: { instanceID: string; instance?: Instance; onBack: () => void }) {
  const host = useRef<HTMLDivElement>(null)
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
    socket.onmessage = event => terminal.write(typeof event.data === 'string' ? event.data : '')
    socket.onerror = () => setError('终端连接失败，请确认实例和节点均处于运行状态。')
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
  return <section className="flex h-dvh flex-col overflow-hidden bg-slate-50 p-2 dark:bg-slate-950">
    <div className="flex shrink-0 items-center justify-between gap-3 pb-2"><FilterTabs value={mode} onChange={setMode} label="实例工作台模式" items={[{ value: 'terminal', label: '终端' }, { value: 'files', label: '文件' }]} />{mode === 'files' && <span className="hidden min-w-0 flex-1 truncate text-right text-[11px] text-slate-500 sm:block">/app/workspace{path ? `/${path}` : ''}</span>}<Button tone="ghost" className="min-h-8 px-2.5" onClick={onBack}>返回</Button></div>
    <main className="min-h-0 flex-1">
      {!ready && mode === 'terminal' ? <Alert tone="info">终端将在实例运行后可用，请等待部署或启动完成。</Alert> : mode === 'terminal' ? <div className="relative h-full"><div ref={host} className="h-full overflow-hidden rounded-xl border border-slate-800 bg-slate-950 p-3 shadow-sm" aria-label="容器终端" />{error && <div className="absolute inset-x-3 bottom-3"><Alert tone="error">{error}</Alert></div>}</div> : <section className="grid h-full min-h-0 grid-cols-[17rem_1fr] overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800 max-[850px]:h-auto max-[850px]:grid-cols-1 max-[850px]:overflow-visible">
        <aside className="flex min-h-0 flex-col border-r border-slate-200 p-3 dark:border-slate-700 max-[850px]:border-r-0 max-[850px]:border-b"><div className="mb-3 flex items-center justify-between gap-2"><p className="m-0 truncate text-xs font-bold">工作区文件</p><label className="cursor-pointer text-[11px] font-bold text-blue-700"><input className="sr-only" type="file" onChange={event => void upload(event.target.files?.[0])} />{uploading ? '上传中…' : '上传'}</label></div>{path && <Button tone="ghost" className="mb-2 self-start" onClick={() => setPath(path.split('/').slice(0, -1).join('/'))}>返回上级</Button>}<div className="min-h-0 flex-1 overflow-y-auto">{filesLoading ? <LoadingState>正在读取文件…</LoadingState> : <div className="space-y-1">{listing?.entries.map(entry => <button key={entry.path} type="button" onClick={() => void openEntry(entry)} className="flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-xs hover:bg-blue-50 dark:hover:bg-slate-700"><span aria-hidden="true">{entry.kind === 'directory' ? '📁' : '📄'}</span><span className="min-w-0 truncate">{entry.name}</span></button>)}</div>}</div></aside>
        <section className="flex min-h-0 flex-col p-3">{editor ? <><div className="mb-2 flex shrink-0 items-center justify-between gap-3"><span className="truncate text-xs font-bold">{editor.path}</span><Button loading={saving} onClick={() => void saveFile({ id: instanceID, ...editor }).unwrap().then(() => void refetch())}>保存</Button></div><textarea aria-label="文件内容" value={editor.content} onChange={event => setEditor({ ...editor, content: event.target.value })} spellCheck={false} className="min-h-0 flex-1 resize-none rounded-lg border border-slate-200 bg-slate-950 p-3 font-mono text-xs leading-6 text-slate-100 outline-none focus:border-blue-500 dark:border-slate-600 max-[850px]:min-h-96" /></> : <EmptyState title="打开一个文件" description={opening ? '正在读取文件…' : '选择左侧工作区文件即可查看与编辑。'} />}</section>
      </section>}
    </main>
  </section>
}
