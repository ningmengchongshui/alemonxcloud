import { useId } from 'react'
import { Alert, Button } from '@/components/ui'

export function TicketReplyComposer({
  title,
  helper,
  placeholder,
  value,
  error,
  sending,
  onChange,
  onSubmit
}: {
  title: string
  helper: string
  placeholder: string
  value: string
  error?: string
  sending: boolean
  onChange: (value: string) => void
  onSubmit: () => void
}) {
  const inputID = useId()
  return (
    <form
      className="mt-6 max-w-3xl overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-700 dark:bg-slate-800"
      onSubmit={event => {
        event.preventDefault()
        onSubmit()
      }}
    >
      <div className="border-b border-slate-100 px-4 py-3.5 dark:border-slate-700">
        <div className="flex items-start justify-between gap-4">
          <div>
            <label
              htmlFor={inputID}
              className="block text-sm font-bold text-slate-800 dark:text-white"
            >
              {title}
            </label>
            <p className="mb-0 mt-1 text-xs leading-5 text-slate-500 dark:text-slate-300">
              {helper}
            </p>
          </div>
          <span className="shrink-0 pt-0.5 text-xs tabular-nums text-slate-400">
            {value.length}/4000
          </span>
        </div>
        <textarea
          id={inputID}
          rows={5}
          maxLength={4000}
          value={value}
          onChange={event => onChange(event.target.value)}
          placeholder={placeholder}
          className="mt-3 block min-h-32 w-full resize-y rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 text-sm leading-6 text-slate-800 outline-none placeholder:text-slate-400 focus:border-blue-500 focus:bg-white focus:ring-3 focus:ring-blue-100 dark:border-slate-600 dark:bg-slate-900 dark:text-white dark:focus:border-blue-400 dark:focus:bg-slate-950 dark:focus:ring-blue-950"
        />
        {error && <Alert tone="error">{error}</Alert>}
      </div>
      <div className="flex flex-wrap items-center justify-between gap-3 bg-slate-50 px-4 py-3 dark:bg-slate-900/40">
        <p className="m-0 text-xs text-slate-500 dark:text-slate-300">
          发送后会同步到当前工单会话。
        </p>
        <Button type="submit" loading={sending} disabled={!value.trim()}>
          发送回复
        </Button>
      </div>
    </form>
  )
}
