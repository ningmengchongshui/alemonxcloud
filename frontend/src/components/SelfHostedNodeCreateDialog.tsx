import { useState } from 'react'
import { Button, Dialog, DialogFooter, dialogFieldClass, dialogLabelClass } from '@/components/ui'
import { useCreateControlEnrollmentTokenMutation } from '@/services/cloudApi'

export function SelfHostedNodeCreateDialog({
  onClose
}: {
  onClose: () => void
}) {
  const [createToken, { isLoading }] = useCreateControlEnrollmentTokenMutation()
  const [name, setName] = useState('')
  const [token, setToken] = useState('')
  const [copied, setCopied] = useState(false)

  function create() {
    if (!name.trim()) return
    void createToken({ name: name.trim() })
      .unwrap()
      .then(result => setToken(result.token))
      .catch(() => undefined)
  }

  function copyToken() {
    void navigator.clipboard.writeText(token).then(() => setCopied(true)).catch(() => undefined)
  }

  return (
    <Dialog
      eyebrow="自建节点"
      title={token ? '保存接入 Token' : '新建节点'}
      description={
        token
          ? 'Token 仅显示一次，请立即保存并按部署说明写入 Agent 配置。'
          : '为要接入的服务器命名后生成一次性 Token。节点连接成功后会自动出现在列表中。'
      }
      onClose={onClose}
    >
      {token ? (
        <div className="space-y-3">
          <label className={dialogLabelClass}>
            接入 Token
            <input readOnly value={token} className={`${dialogFieldClass} font-mono text-xs`} />
          </label>
          <p className="m-0 rounded-lg bg-amber-50 px-3 py-2.5 text-xs leading-5 text-amber-800 dark:bg-amber-950/50 dark:text-amber-100">
            此 Token 10 分钟内有效。请勿发送给他人，过期后需重新创建节点。
          </p>
        </div>
      ) : (
        <label className={dialogLabelClass}>
          节点名称
          <input
            data-autofocus
            value={name}
            onChange={event => setName(event.target.value)}
            maxLength={64}
            placeholder="例如：香港云主机"
            className={dialogFieldClass}
          />
        </label>
      )}
      <DialogFooter>
        <Button tone="secondary" onClick={onClose}>
          {token ? '完成' : '取消'}
        </Button>
        {token ? (
          <Button onClick={copyToken}>{copied ? '已复制' : '复制 Token'}</Button>
        ) : (
          <Button loading={isLoading} disabled={!name.trim()} onClick={create}>
            生成 Token
          </Button>
        )}
      </DialogFooter>
    </Dialog>
  )
}
