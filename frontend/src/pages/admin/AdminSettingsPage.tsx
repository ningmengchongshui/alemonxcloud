import { useEffect, useState } from 'react'
import { Alert, Button, LoadingState, PageHeader } from '@/components/ui'
import {
  useGetAdminRechargeContactQuery,
  useSaveAdminRechargeContactMutation
} from '@/services/cloudApi'

const inputClass =
  'mt-1 block w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-800 outline-none placeholder:text-slate-400 focus:border-blue-500 dark:border-slate-600 dark:bg-slate-900 dark:text-white'

export function AdminSettingsPage() {
  const contact = useGetAdminRechargeContactQuery()
  const [save, { isLoading: saving }] = useSaveAdminRechargeContactMutation()
  const [name, setName] = useState('')
  const [url, setURL] = useState('')
  const [error, setError] = useState('')
  useEffect(() => {
    if (contact.data) {
      setName(contact.data.name)
      setURL(contact.data.url)
    }
  }, [contact.data])
  if (contact.isLoading)
    return (
      <section className="page super-page">
        <LoadingState>正在加载平台设置…</LoadingState>
      </section>
    )
  return (
    <section className="page super-page">
      <PageHeader
        title="平台设置"
        description="管理用户端可见的联系与服务信息。"
      />
      <section className="max-w-2xl rounded-xl border border-slate-200 bg-white p-5 dark:border-slate-700 dark:bg-slate-800">
        <h2 className="m-0 text-base font-bold text-slate-900 dark:text-white">
          人工充值咨询
        </h2>
        <p className="mt-1 text-xs leading-5 text-slate-500 dark:text-slate-300">
          用户在账户卡中点击充值后，会看到此处配置的咨询群名称和地址。
        </p>
        <form
          className="mt-5 space-y-4"
          onSubmit={event => {
            event.preventDefault()
            setError('')
            void save({ name: name.trim(), url: url.trim() })
              .unwrap()
              .catch(value =>
                setError(
                  typeof value?.data?.message === 'string'
                    ? value.data.message
                    : '保存失败，请稍后重试'
                )
              )
          }}
        >
          <label className="block text-[11px] font-bold text-slate-700 dark:text-slate-100">
            咨询群名称
            <input
              className={inputClass}
              value={name}
              onChange={event => setName(event.target.value)}
              placeholder="例如：ALemonX 售前咨询群"
              data-autofocus
            />
          </label>
          <label className="block text-[11px] font-bold text-slate-700 dark:text-slate-100">
            咨询地址
            <input
              className={inputClass}
              type="url"
              value={url}
              onChange={event => setURL(event.target.value)}
              placeholder="https://example.com/group"
            />
          </label>
          {error && <Alert tone="error">{error}</Alert>}
          <div className="flex justify-end">
            <Button
              type="submit"
              loading={saving}
              disabled={!name.trim() || !url.trim()}
            >
              保存配置
            </Button>
          </div>
        </form>
      </section>
    </section>
  )
}
