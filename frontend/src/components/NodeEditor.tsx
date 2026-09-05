import { useState, type ReactNode } from 'react'
import { useSaveAdminNodeMutation } from '@/services/cloudApi'
import type { Node } from '@/types/cloud'
import { Alert, Button, Dialog } from '@/components/ui'

const inputClass =
  'mt-1 block w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-800 outline-none placeholder:text-slate-400 focus:border-blue-500 dark:border-slate-600 dark:bg-slate-900 dark:text-white'

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block text-[11px] font-bold text-slate-700 dark:text-slate-100">
      {label}
      {children}
    </label>
  )
}

export function NodeEditor() {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [agentURL, setAgentURL] = useState('')
  const [token, setToken] = useState('')
  const [error, setError] = useState('')
  const [save, { isLoading }] = useSaveAdminNodeMutation()
  const close = () => {
    setOpen(false)
    setError('')
  }
  async function register() {
    try {
      setError('')
      await save({
        id: '',
        name: name.trim(),
        agentURL: agentURL.trim(),
        agentToken: token,
        cpuTotal: 0,
        memoryTotalMB: 0,
        cpuDetected: 0,
        memoryDetectedMB: 0,
        cpuReserved: 0,
        memoryReservedMB: 0,
        enabled: false
      }).unwrap()
      setName('')
      setAgentURL('')
      setToken('')
      close()
    } catch {
      setError('Agent 验证或硬件探测失败，请检查地址和一次性令牌。')
    }
  }
  return (
    <>
      <Button onClick={() => setOpen(true)}>＋ 注册新节点</Button>
      {open && (
        <Dialog title="注册新节点" onClose={close}>
          <div className="space-y-4">
            <Field label="节点名称">
              <input
                className={inputClass}
                value={name}
                onChange={event => setName(event.target.value)}
                autoFocus
              />
            </Field>
            <Field label="Agent 地址">
              <input
                className={inputClass}
                value={agentURL}
                onChange={event => setAgentURL(event.target.value)}
                placeholder="http://10.0.0.12:13092"
              />
            </Field>
            <Field label="一次性 Agent 令牌">
              <input
                className={inputClass}
                type="password"
                value={token}
                onChange={event => setToken(event.target.value)}
                autoComplete="new-password"
              />
            </Field>
            {error && <Alert tone="error">{error}</Alert>}
            <div className="flex justify-end gap-2">
              <Button tone="secondary" onClick={close}>
                取消
              </Button>
              <Button
                loading={isLoading}
                disabled={!name.trim() || !agentURL.trim() || !token.trim()}
                onClick={() => void register()}
              >
                验证并注册
              </Button>
            </div>
          </div>
        </Dialog>
      )}
    </>
  )
}

export function NodeConfigButton({ node }: { node: Node }) {
  const [open, setOpen] = useState(false)
  const [agentURL, setAgentURL] = useState(node.agentURL)
  const [enabled, setEnabled] = useState(node.enabled)
  const [cpu, setCPU] = useState(node.cpuTotal)
  const [memory, setMemory] = useState(node.memoryTotalMB)
  const [error, setError] = useState('')
  const [save, { isLoading }] = useSaveAdminNodeMutation()
  const detectedCPU = node.cpuDetected || node.cpuTotal
  const detectedMemory = node.memoryDetectedMB || node.memoryTotalMB
  const invalidCapacity =
    cpu <= 0 || memory < 256 || cpu > detectedCPU || memory > detectedMemory
  const close = () => {
    setOpen(false)
    setError('')
  }
  function openEditor() {
    setAgentURL(node.agentURL)
    setEnabled(node.enabled)
    setCPU(node.cpuTotal)
    setMemory(node.memoryTotalMB)
    setError('')
    setOpen(true)
  }
  async function saveConfig() {
    try {
      await save({
        ...node,
        agentURL: agentURL.trim(),
        enabled,
        cpuTotal: cpu,
        memoryTotalMB: memory
      }).unwrap()
      close()
    } catch {
      setError('节点配置保存失败，请稍后重试')
    }
  }
  return (
    <>
      <button
        type="button"
        className="text-[11px] font-bold text-blue-700 hover:underline dark:text-blue-200"
        onClick={openEditor}
      >
        配置
      </button>
      {open && (
        <Dialog
          title={node.name}
          description="可调度容量不能超过 Agent 自动识别的物理硬件。"
          onClose={close}
        >
          <div className="space-y-4">
            <div className="rounded-lg bg-blue-50 p-3 text-[11px] text-blue-800 dark:bg-blue-950 dark:text-blue-100">
              <b className="block">识别硬件</b>
              <span>
                {detectedCPU} 核 CPU · {Math.round(detectedMemory / 1024)} GB
                内存
              </span>
            </div>
            <Field label="Agent 地址">
              <input
                className={inputClass}
                value={agentURL}
                onChange={event => setAgentURL(event.target.value)}
              />
            </Field>
            <div className="grid grid-cols-2 gap-3 max-[560px]:grid-cols-1">
              <Field label={`可调度 CPU（最多 ${detectedCPU} 核）`}>
                <input
                  className={inputClass}
                  type="number"
                  min="0.1"
                  max={detectedCPU}
                  step="0.1"
                  value={cpu}
                  onChange={event => setCPU(Number(event.target.value))}
                />
              </Field>
              <Field label={`可调度内存 MB（最多 ${detectedMemory}）`}>
                <input
                  className={inputClass}
                  type="number"
                  min="256"
                  max={detectedMemory}
                  value={memory}
                  onChange={event => setMemory(Number(event.target.value))}
                />
              </Field>
            </div>
            <Field label="节点状态">
              <select
                className={inputClass}
                value={enabled ? 'enabled' : 'disabled'}
                onChange={event => setEnabled(event.target.value === 'enabled')}
              >
                <option value="disabled">停用</option>
                <option value="enabled">启用并参与调度</option>
              </select>
            </Field>
            {enabled && invalidCapacity && (
              <Alert tone="error">
                启用节点前，请将可调度 CPU 和内存调整到检测到的硬件范围内。
              </Alert>
            )}
            {error && <Alert tone="error">{error}</Alert>}
            <div className="flex justify-end gap-2">
              <Button tone="secondary" onClick={close}>
                取消
              </Button>
              <Button
                loading={isLoading}
                disabled={!agentURL.trim() || (enabled && invalidCapacity)}
                onClick={() => void saveConfig()}
              >
                保存配置
              </Button>
            </div>
          </div>
        </Dialog>
      )}
    </>
  )
}
