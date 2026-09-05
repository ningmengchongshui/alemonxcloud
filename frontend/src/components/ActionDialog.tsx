import type { ReactNode } from 'react'
import {
  Button,
  Dialog,
  DialogFooter,
  dialogFieldClass,
  dialogLabelClass
} from '@/components/ui'

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
  children?: ReactNode
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
  busy,
  children
}: ActionDialogProps) {
  return (
    <Dialog title={title} description={description} onClose={onCancel}>
      {inputLabel && (
        <label
          className={`mb-4 ${dialogLabelClass}`}
          htmlFor="action-dialog-input"
        >
          {inputLabel}
          <input
            id="action-dialog-input"
            data-autofocus
            value={inputValue ?? ''}
            onChange={event => onInputChange?.(event.target.value)}
            placeholder={inputPlaceholder}
            className={dialogFieldClass}
          />
        </label>
      )}
      {secondaryInputLabel && (
        <label className={`mb-4 ${dialogLabelClass}`}>
          {secondaryInputLabel}
          <input
            value={secondaryInputValue ?? ''}
            onChange={event => onSecondaryInputChange?.(event.target.value)}
            placeholder={secondaryInputPlaceholder}
            className={dialogFieldClass}
          />
        </label>
      )}
      {children}
      <DialogFooter>
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
      </DialogFooter>
    </Dialog>
  )
}
