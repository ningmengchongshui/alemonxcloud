import { useState } from 'react'
import {
  useGetAdminCatalogQuery,
  useSaveAdminImageMutation
} from '@/services/cloudApi'
import { Alert, Button, Dialog } from '@/components/ui'

const inputClass =
  'mt-1 block w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-800 outline-none placeholder:text-slate-400 focus:border-blue-500 dark:border-slate-600 dark:bg-slate-900 dark:text-white'

export function ImageSourceEditor() {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [ref, setRef] = useState('')
  const [error, setError] = useState('')
  const { data: catalog } = useGetAdminCatalogQuery()
  const [save, { isLoading }] = useSaveAdminImageMutation()
  const close = () => {
    setOpen(false)
    setError('')
  }
  async function submit() {
    const imageRef = ref.trim()
    if (
      catalog?.images.some(
        image => image.imageRef.trim().toLowerCase() === imageRef.toLowerCase()
      )
    ) {
      setError('镜像地址已存在，请勿重复添加。')
      return
    }
    try {
      await save({
        id: '',
        name: name.trim(),
        imageRef,
        imageDigest: '',
        version: 'latest',
        enabled: true
      }).unwrap()
      setName('')
      setRef('')
      close()
    } catch {
      setError('镜像来源保存失败，请确认地址格式后重试。')
    }
  }
  return (
    <>
      <Button tone="secondary" onClick={() => setOpen(true)}>
        ＋ 新增镜像来源
      </Button>
      {open && (
        <Dialog
          title="新增镜像来源"
          description="登记可信镜像地址后，平台会以受控版本提供给用户选择。"
          onClose={close}
        >
          <div className="space-y-4">
            <label
              className="block text-[11px] font-bold text-slate-700 dark:text-slate-100"
              htmlFor="image-source-name"
            >
              镜像名称
              <input
                id="image-source-name"
                className={inputClass}
                value={name}
                onChange={event => setName(event.target.value)}
                placeholder="AlemonX"
                data-autofocus
              />
            </label>
            <label
              className="block text-[11px] font-bold text-slate-700 dark:text-slate-100"
              htmlFor="image-source-ref"
            >
              镜像地址
              <input
                id="image-source-ref"
                className={inputClass}
                value={ref}
                onChange={event => setRef(event.target.value)}
                placeholder="registry.example/alemonx"
              />
            </label>
            {error && <Alert tone="error">{error}</Alert>}
            <div className="flex justify-end gap-2">
              <Button tone="secondary" onClick={close}>
                取消
              </Button>
              <Button
                loading={isLoading}
                disabled={!name.trim() || !ref.trim()}
                onClick={() => void submit()}
              >
                保存镜像来源
              </Button>
            </div>
          </div>
        </Dialog>
      )}
    </>
  )
}
