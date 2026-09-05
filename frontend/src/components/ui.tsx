import {
  useEffect,
  useId,
  useRef,
  type ButtonHTMLAttributes,
  type PropsWithChildren,
  type ReactNode
} from 'react'
import classNames from 'classnames'

type ButtonTone = 'primary' | 'secondary' | 'danger' | 'ghost'

export function Button({
  children,
  className,
  tone = 'primary',
  loading,
  disabled,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  tone?: ButtonTone
  loading?: boolean
}) {
  const toneClass: Record<ButtonTone, string> = {
    primary:
      'bg-blue-600 text-white shadow-sm shadow-blue-500/25 hover:bg-blue-700',
    secondary:
      'border border-slate-200 bg-white text-slate-700 hover:border-blue-300 hover:bg-blue-50 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-100 dark:hover:bg-slate-700',
    danger:
      'border border-red-200 bg-red-50 text-red-700 hover:bg-red-100 dark:border-red-900 dark:bg-red-950 dark:text-red-200',
    ghost:
      'bg-transparent text-blue-700 hover:bg-blue-50 dark:text-blue-200 dark:hover:bg-blue-950'
  }
  return (
    <button
      {...props}
      disabled={disabled || loading}
      className={classNames(
        'inline-flex min-h-10 items-center justify-center gap-2 rounded-md px-3.5 text-xs font-bold transition-colors focus-visible:outline-3 focus-visible:outline-offset-2 focus-visible:outline-blue-200 disabled:cursor-not-allowed disabled:opacity-60',
        toneClass[tone],
        className
      )}
    >
      {loading && (
        <span
          className="size-3 animate-spin rounded-full border-2 border-current border-r-transparent"
          aria-hidden="true"
        />
      )}
      {children}
    </button>
  )
}

export function StatusBadge({
  tone = 'neutral',
  children,
  className
}: PropsWithChildren<{
  tone?: 'success' | 'pending' | 'danger' | 'neutral' | 'progress'
  className?: string
}>) {
  const tones = {
    success:
      'bg-emerald-50 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-200',
    pending: 'bg-amber-50 text-amber-700 dark:bg-amber-950 dark:text-amber-200',
    progress: 'bg-blue-50 text-blue-700 dark:bg-blue-950 dark:text-blue-200',
    danger: 'bg-red-50 text-red-700 dark:bg-red-950 dark:text-red-200',
    neutral: 'bg-slate-100 text-slate-600 dark:bg-slate-700 dark:text-slate-200'
  }
  return (
    <span
      className={classNames(
        'inline-flex items-center gap-1.5 rounded-full px-2 py-1 text-[10px] font-extrabold',
        tones[tone],
        className
      )}
    >
      <i className="size-1.5 rounded-full bg-current" aria-hidden="true" />
      {children}
    </span>
  )
}

export function Alert({
  tone = 'info',
  children
}: PropsWithChildren<{ tone?: 'info' | 'error' | 'success' }>) {
  const tones = {
    info: 'border-blue-200 bg-blue-50 text-blue-800 dark:border-blue-900 dark:bg-blue-950 dark:text-blue-100',
    error:
      'border-red-200 bg-red-50 text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-200',
    success:
      'border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950 dark:text-emerald-100'
  }
  return (
    <div
      className={classNames(
        'my-3 rounded-lg border px-3 py-2.5 text-[11px] leading-5',
        tones[tone]
      )}
      role={tone === 'error' ? 'alert' : 'status'}
    >
      {children}
    </div>
  )
}

export function LoadingState({
  children = '正在加载数据…'
}: {
  children?: ReactNode
}) {
  return (
    <div className="flex min-h-44 items-center justify-center gap-2.5 rounded-xl border border-slate-200 bg-white p-6 text-xs text-slate-500 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300">
      <span
        className="size-3 animate-pulse rounded-full bg-blue-500"
        aria-hidden="true"
      />
      {children}
    </div>
  )
}

export function EmptyState({
  title,
  description,
  action
}: {
  title: string
  description: string
  action?: ReactNode
}) {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center rounded-xl border border-dashed border-slate-300 bg-white p-8 text-center dark:border-slate-600 dark:bg-slate-800">
      <span
        className="mb-4 grid size-10 place-items-center rounded-xl bg-blue-50 text-xl text-blue-600 dark:bg-blue-950 dark:text-blue-200"
        aria-hidden="true"
      >
        ＋
      </span>
      <h2 className="m-0 text-lg font-bold text-slate-800 dark:text-white">
        {title}
      </h2>
      <p className="my-2 max-w-sm text-xs leading-5 text-slate-500 dark:text-slate-300">
        {description}
      </p>
      {action}
    </div>
  )
}

export function PageHeader({
  eyebrow,
  title,
  description,
  actions
}: {
  eyebrow?: string
  title: string
  description: string
  actions?: ReactNode
}) {
  return (
    <header className="mb-6 flex items-start justify-between gap-5 max-[850px]:flex-col max-[850px]:items-stretch">
      <div>
        {eyebrow && (
          <p className="mb-1.5 text-[10px] font-extrabold tracking-widest text-blue-600">
            {eyebrow}
          </p>
        )}
        <h1 className="mb-1.5 text-2xl font-bold tracking-tight text-slate-900 dark:text-white">
          {title}
        </h1>
        <p className="m-0 max-w-2xl text-[13px] leading-6 text-slate-500 dark:text-slate-300">
          {description}
        </p>
      </div>
      {actions && (
        <div className="flex shrink-0 items-center gap-2 max-[850px]:w-full">
          {actions}
        </div>
      )}
    </header>
  )
}

export function FilterTabs<T extends string>({
  value,
  items,
  onChange,
  label
}: {
  value: T
  items: Array<{ value: T; label: string }>
  onChange: (value: T) => void
  label: string
}) {
  return (
    <div
      className="flex flex-wrap gap-1 rounded-lg bg-slate-100 p-1 dark:bg-slate-900"
      role="tablist"
      aria-label={label}
    >
      {items.map(item => (
        <button
          key={item.value}
          type="button"
          role="tab"
          aria-selected={value === item.value}
          onClick={() => onChange(item.value)}
          className={classNames(
            'rounded-md px-3 py-1.5 text-[10px] font-bold transition-colors focus-visible:outline-2 focus-visible:outline-blue-500',
            value === item.value
              ? 'bg-white text-blue-700 shadow-sm dark:bg-slate-700 dark:text-blue-200'
              : 'text-slate-500 hover:text-blue-700 dark:text-slate-300 dark:hover:text-blue-200'
          )}
        >
          {item.label}
        </button>
      ))}
    </div>
  )
}

export function InlineAction({
  children,
  className,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      {...props}
      className={classNames(
        'border-0 bg-transparent p-0 text-[11px] font-bold text-blue-700 hover:underline focus-visible:outline-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-200',
        className
      )}
    >
      {children}
    </button>
  )
}

export function DataTable({
  title,
  description,
  children,
  actions
}: PropsWithChildren<{
  title?: string
  description?: string
  actions?: ReactNode
}>) {
  return (
    <section className="overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-800">
      <div className="flex items-start justify-between gap-4 border-b border-slate-100 px-5 py-4 dark:border-slate-700 max-[560px]:flex-col">
        <div>
          {title && (
            <h2 className="m-0 text-sm font-bold text-slate-800 dark:text-white">
              {title}
            </h2>
          )}
          {description && (
            <p className="mt-1 text-[11px] leading-5 text-slate-500 dark:text-slate-300">
              {description}
            </p>
          )}
        </div>
        {actions && <div className="shrink-0">{actions}</div>}
      </div>
      <div className="overflow-x-auto">
        <table className="min-w-full text-left text-xs">{children}</table>
      </div>
    </section>
  )
}

export function Dialog({
  title,
  eyebrow = '',
  description,
  children,
  onClose,
  labelledBy,
  className
}: PropsWithChildren<{
  title: string
  eyebrow?: string
  description?: string
  onClose: () => void
  labelledBy?: string
  className?: string
}>) {
  const dialogRef = useRef<HTMLElement>(null)
  const previousFocusRef = useRef<HTMLElement | null>(null)
  const onCloseRef = useRef(onClose)
  const generatedID = useId()
  const titleID = labelledBy ?? generatedID
  onCloseRef.current = onClose
  useEffect(() => {
    previousFocusRef.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null
    const timer = window.setTimeout(
      () =>
        dialogRef.current
          ?.querySelector<HTMLElement>(
            '[data-autofocus], button, input, select, textarea, [tabindex]:not([tabindex="-1"])'
          )
          ?.focus(),
      0
    )
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        onCloseRef.current()
        return
      }
      if (event.key !== 'Tab' || !dialogRef.current) return
      const focusable = Array.from(
        dialogRef.current.querySelectorAll<HTMLElement>(
          'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
        )
      )
      if (!focusable.length) return
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      }
      if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => {
      window.clearTimeout(timer)
      document.removeEventListener('keydown', onKeyDown)
      previousFocusRef.current?.focus()
    }
  }, [])
  return (
    <div
      className="fixed inset-0 z-50 grid place-items-center bg-slate-950/45 p-4 backdrop-blur-sm"
      onMouseDown={event => {
        if (event.target === event.currentTarget) onCloseRef.current()
      }}
    >
      <section
        ref={dialogRef}
        className={classNames(
          'w-full max-w-lg rounded-xl border border-slate-200 bg-white p-5 shadow-2xl dark:border-slate-700 dark:bg-slate-800',
          className
        )}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleID}
      >
        <div className="mb-5 flex items-start justify-between gap-4">
          <div>
            {eyebrow && (
              <p className="mb-2 text-[10px] font-extrabold tracking-widest text-blue-600">
                {eyebrow}
              </p>
            )}
            <h2
              id={titleID}
              className="m-0 text-lg font-bold text-slate-800 dark:text-white"
            >
              {title}
            </h2>
            {description && (
              <p className="mt-2 text-xs leading-5 text-slate-500 dark:text-slate-300">
                {description}
              </p>
            )}
          </div>
          <button
            type="button"
            onClick={() => onCloseRef.current()}
            className="grid size-8 place-items-center rounded-md border-0 bg-slate-100 text-lg text-slate-500 hover:bg-slate-200 dark:bg-slate-700 dark:text-slate-200"
            aria-label="关闭对话框"
          >
            ×
          </button>
        </div>
        {children}
      </section>
    </div>
  )
}

export const dialogLabelClass =
  'block text-[11px] font-bold text-slate-700 dark:text-slate-100'

export const dialogFieldClass =
  'mt-1.5 block w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm font-normal text-slate-800 outline-none placeholder:text-slate-400 focus:border-blue-500 focus:ring-3 focus:ring-blue-100 dark:border-slate-600 dark:bg-slate-900 dark:text-white dark:focus:border-blue-400 dark:focus:ring-blue-950'

export function DialogFooter({ children }: PropsWithChildren) {
  return (
    <div className="flex flex-wrap justify-end gap-2 border-t border-slate-100 pt-4 dark:border-slate-700">
      {children}
    </div>
  )
}
