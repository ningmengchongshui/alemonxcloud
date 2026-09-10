import { useState } from 'react'
import {
  Button,
  Dialog,
  DialogFooter,
  dialogFieldClass,
  dialogLabelClass
} from '@/components/ui'
import { useUpdateSelfHostedNodeMutation } from '@/services/cloudApi'

export function SelfHostedNodeRenameDialog({
  id,
  name,
  onClose
}: {
  id: string
  name: string
  onClose: () => void
}) {
  const [nextName, setNextName] = useState(name)
  const [rename, { isLoading }] = useUpdateSelfHostedNodeMutation()

  function submit() {
    const value = nextName.trim()
    if (!value || value === name) return
    void rename({ id, name: value })
      .unwrap()
      .then(onClose)
      .catch(() => undefined)
  }

  return (
    <Dialog
      eyebrow="节点设置"
      title="修改节点名称"
      description="名称只用于控制台识别，不会影响 Agent 连接或已经部署的实例。"
      onClose={onClose}
    >
      <label className={dialogLabelClass}>
        节点名称
        <input
          data-autofocus
          value={nextName}
          onChange={event => setNextName(event.target.value)}
          maxLength={64}
          className={dialogFieldClass}
        />
      </label>
      <DialogFooter>
        <Button tone="secondary" disabled={isLoading} onClick={onClose}>
          取消
        </Button>
        <Button
          loading={isLoading}
          disabled={!nextName.trim() || nextName.trim() === name}
          onClick={submit}
        >
          保存
        </Button>
      </DialogFooter>
    </Dialog>
  )
}
