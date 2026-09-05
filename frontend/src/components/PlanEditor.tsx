import { useState } from 'react'
import { useSaveAdminPlanMutation } from '@/services/cloudApi'
import { Alert, Button, Dialog } from '@/components/ui'

const inputClass =
  'mt-1 block w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-800 outline-none placeholder:text-slate-400 focus:border-blue-500 dark:border-slate-600 dark:bg-slate-900 dark:text-white'

export function PlanEditor() {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [cpu, setCPU] = useState(2)
  const [memory, setMemory] = useState(4096)
  const [price, setPrice] = useState(9900)
  const [error, setError] = useState('')
  const [save, { isLoading }] = useSaveAdminPlanMutation()
  const close = () => {
    setOpen(false)
    setError('')
  }
  async function submit() {
    try {
      await save({
        id: '',
        name: name.trim(),
        cpu,
        memoryMB: memory,
        monthlyPriceFen: price,
        enabled: true,
        sortOrder: 100
      }).unwrap()
      setName('')
      close()
    } catch {
      setError('计算套餐保存失败，请检查参数后重试。')
    }
  }
  return (
    <>
      <Button onClick={() => setOpen(true)}>＋ 新增计算套餐</Button>
      {open && (
        <Dialog
          title="新增计算套餐"
          description="配置用户可购买的 CPU、内存与月度价格。"
          onClose={close}
        >
          <div className="space-y-4">
            <label
              className="block text-[11px] font-bold text-slate-700 dark:text-slate-100"
              htmlFor="plan-name"
            >
              套餐名称
              <input
                id="plan-name"
                className={inputClass}
                value={name}
                onChange={event => setName(event.target.value)}
                placeholder="标准版"
                data-autofocus
              />
            </label>
            <div className="grid grid-cols-2 gap-3 max-[560px]:grid-cols-1">
              <label
                className="block text-[11px] font-bold text-slate-700 dark:text-slate-100"
                htmlFor="plan-cpu"
              >
                CPU 核数
                <input
                  id="plan-cpu"
                  className={inputClass}
                  type="number"
                  min="1"
                  value={cpu}
                  onChange={event => setCPU(Number(event.target.value))}
                />
              </label>
              <label
                className="block text-[11px] font-bold text-slate-700 dark:text-slate-100"
                htmlFor="plan-memory"
              >
                内存 MB
                <input
                  id="plan-memory"
                  className={inputClass}
                  type="number"
                  min="256"
                  value={memory}
                  onChange={event => setMemory(Number(event.target.value))}
                />
              </label>
            </div>
            <label
              className="block text-[11px] font-bold text-slate-700 dark:text-slate-100"
              htmlFor="plan-price"
            >
              月价（分）
              <input
                id="plan-price"
                className={inputClass}
                type="number"
                min="0"
                value={price}
                onChange={event => setPrice(Number(event.target.value))}
              />
            </label>
            {error && <Alert tone="error">{error}</Alert>}
            <div className="flex justify-end gap-2">
              <Button tone="secondary" onClick={close}>
                取消
              </Button>
              <Button
                loading={isLoading}
                disabled={!name.trim() || cpu < 1 || memory < 256 || price < 0}
                onClick={() => void submit()}
              >
                保存计算套餐
              </Button>
            </div>
          </div>
        </Dialog>
      )}
    </>
  )
}
