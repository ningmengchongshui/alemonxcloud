import { useEffect, useState } from 'react'
import { useSaveAdminNodeMutation } from '@/services/cloudApi'
import type { Node } from '@/types/cloud'

function useEscape(close: () => void) {
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => { if (event.key === 'Escape') close() }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [close])
}

export function NodeEditor() {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [agentURL, setAgentURL] = useState('')
  const [token, setToken] = useState('')
  const [error, setError] = useState('')
  const [save, { isLoading }] = useSaveAdminNodeMutation()
  useEscape(() => setOpen(false))

  async function register() {
    try {
      setError('')
      await save({ id: '', name, agentURL, agentToken: token, cpuTotal: 0, memoryTotalMB: 0, cpuDetected: 0, memoryDetectedMB: 0, cpuReserved: 0, memoryReservedMB: 0, enabled: false }).unwrap()
      setName(''); setAgentURL(''); setToken(''); setOpen(false)
    } catch { setError('Agent 验证或硬件探测失败') }
  }

  return <><button className="primary" onClick={() => { setError(''); setOpen(true) }}>＋ 注册新节点</button>{open && <div className="modal-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) setOpen(false) }}><section className="modal-card compact-modal node-editor" role="dialog" aria-modal="true" aria-labelledby="register-node-title"><div className="modal-heading"><div><p className="eyebrow">节点管理</p><h2 id="register-node-title">注册新节点</h2><p>验证 Agent 后，自动识别硬件并加入调度。</p></div><button className="modal-close" onClick={() => setOpen(false)} aria-label="关闭注册节点弹窗">×</button></div><label>节点名称<input value={name} onChange={event => setName(event.target.value)} /></label><label>Agent 地址<input value={agentURL} onChange={event => setAgentURL(event.target.value)} placeholder="http://10.0.0.12:13092" /></label><label>一次性 Agent 令牌<input type="password" value={token} onChange={event => setToken(event.target.value)} autoComplete="new-password" /></label><p className="modal-note">CPU、内存和磁盘信息由 Agent 自动识别。</p><div className="modal-actions"><button className="subtle-button" onClick={() => setOpen(false)}>取消</button><button className="primary" disabled={isLoading || !name.trim() || !agentURL.trim() || !token.trim()} onClick={() => void register()}>{isLoading ? '验证中…' : '验证并注册'}</button></div>{error && <p className="login-error" role="alert">{error}</p>}</section></div>}</>
}

export function NodeConfigButton({ node }: { node: Node }) {
  const [open, setOpen] = useState(false)
  const [agentURL, setAgentURL] = useState(node.agentURL)
  const [enabled, setEnabled] = useState(node.enabled)
  const [cpu, setCPU] = useState(node.cpuTotal)
  const [memory, setMemory] = useState(node.memoryTotalMB)
  const [error, setError] = useState('')
  const [save, { isLoading }] = useSaveAdminNodeMutation()
  useEscape(() => setOpen(false))

  async function saveConfig() {
    try { setError(''); await save({ ...node, agentURL, enabled, cpuTotal: cpu, memoryTotalMB: memory }).unwrap(); setOpen(false) }
    catch { setError('节点配置保存失败') }
  }

  const detectedCPU = node.cpuDetected || node.cpuTotal
  const detectedMemory = node.memoryDetectedMB || node.memoryTotalMB
  const invalidCapacity = cpu <= 0 || memory < 256 || cpu > detectedCPU || memory > detectedMemory
  return <><button className="text-button" onClick={() => { setAgentURL(node.agentURL); setEnabled(node.enabled); setCPU(node.cpuTotal); setMemory(node.memoryTotalMB); setError(''); setOpen(true) }}>配置</button>{open && <div className="modal-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) setOpen(false) }}><section className="modal-card compact-modal node-editor" role="dialog" aria-modal="true" aria-labelledby={`node-config-${node.id}`}><div className="modal-heading"><div><p className="eyebrow">节点设置</p><h2 id={`node-config-${node.id}`}>{node.name}</h2><p>确认可调度容量后再启用节点。</p></div><button className="modal-close" onClick={() => setOpen(false)} aria-label="关闭节点设置弹窗">×</button></div><div className="node-hardware"><span>识别硬件</span><b>{detectedCPU} 核 CPU · {Math.round(detectedMemory / 1024)} GB 内存</b></div><label htmlFor={`node-agent-${node.id}`}>Agent 地址<input id={`node-agent-${node.id}`} value={agentURL} onChange={event => setAgentURL(event.target.value)} /></label><div className="admin-editor-fields"><label>可调度 CPU（最多 {detectedCPU} 核）<input type="number" min="0.1" max={detectedCPU} step="0.1" value={cpu} onChange={event => setCPU(Number(event.target.value))} /></label><label>可调度内存 MB（最多 {detectedMemory}）<input type="number" min="256" max={detectedMemory} value={memory} onChange={event => setMemory(Number(event.target.value))} /></label></div><label htmlFor={`node-enabled-${node.id}`}>状态<select id={`node-enabled-${node.id}`} value={enabled ? 'enabled' : 'disabled'} onChange={event => setEnabled(event.target.value === 'enabled')}><option value="disabled">停用</option><option value="enabled">启用并参与调度</option></select></label><div className="modal-actions"><button className="subtle-button" onClick={() => setOpen(false)}>取消</button><button className="primary" disabled={isLoading || !agentURL.trim() || (enabled && invalidCapacity)} onClick={() => void saveConfig()}>{isLoading ? '保存中…' : '保存配置'}</button></div>{error && <p className="login-error" role="alert">{error}</p>}</section></div>}</>
}
