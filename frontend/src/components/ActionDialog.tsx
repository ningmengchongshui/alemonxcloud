import { Button, Dialog } from '@/components/ui'

interface ActionDialogProps {
  title: string
  description: string
  confirmLabel: string
  danger?: boolean
  inputLabel?: string
  inputValue?: string
  inputPlaceholder?: string
  onInputChange?: (value: string) => void
  secondaryInputLabel?: string
  secondaryInputValue?: string
  secondaryInputPlaceholder?: string
  onSecondaryInputChange?: (value: string) => void
  onConfirm: () => void
  onCancel: () => void
  busy?: boolean
}

export function ActionDialog({
  title,
  description,
  confirmLabel,
  danger,
  inputLabel,
  inputValue,
  inputPlaceholder,
  onInputChange,
  secondaryInputLabel,
  secondaryInputValue,
  secondaryInputPlaceholder,
  onSecondaryInputChange,
  onConfirm,
  onCancel,
  busy
}: ActionDialogProps) {
  return (
    <Dialog title={title} description={description} onClose={onCancel}>
      {inputLabel && (
        <label
          className="mb-4 block text-[11px] font-bold text-slate-700 dark:text-slate-100"
          htmlFor="action-dialog-input"
        >
          {inputLabel}
          <input
            id="action-dialog-input"
            data-autofocus
            value={inputValue ?? ''}
            onChange={event => onInputChange?.(event.target.value)}
            placeholder={inputPlaceholder}
            className="mt-2 block w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm font-normal text-slate-800 outline-none placeholder:text-slate-400 focus:border-blue-500 dark:border-slate-600 dark:bg-slate-900 dark:text-white"
          />
        </label>
      )}
      {secondaryInputLabel && (
        <label className="mb-4 block text-[11px] font-bold text-slate-700 dark:text-slate-100">
          {secondaryInputLabel}
          <input
            value={secondaryInputValue ?? ''}
            onChange={event => onSecondaryInputChange?.(event.target.value)}
            placeholder={secondaryInputPlaceholder}
            className="mt-2 block w-full rounded-md border border-slate-300 bg-white px-3 py-2.5 text-sm font-normal text-slate-800 outline-none placeholder:text-slate-400 focus:border-blue-500 dark:border-slate-600 dark:bg-slate-900 dark:text-white"
          />
        </label>
      )}
      <div className="flex justify-end gap-2">
        <Button tone="secondary" onClick={onCancel}>
          取消
        </Button>
        <Button
          tone={danger ? 'danger' : 'primary'}
          loading={busy}
          disabled={Boolean(inputLabel && !inputValue?.trim())}
          onClick={onConfirm}
        >
          {confirmLabel}
        </Button>
      </div>
    </Dialog>
  )
}
