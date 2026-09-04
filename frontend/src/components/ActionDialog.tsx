interface ActionDialogProps {
  title: string
  description: string
  confirmLabel: string
  danger?: boolean
  inputLabel?: string
  inputValue?: string
  inputPlaceholder?: string
  onInputChange?: (value: string) => void
  onConfirm: () => void
  onCancel: () => void
  busy?: boolean
}

export function ActionDialog({ title, description, confirmLabel, danger, inputLabel, inputValue, inputPlaceholder, onInputChange, onConfirm, onCancel, busy }: ActionDialogProps) {
  return <div className="modal-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) onCancel() }}><section className="modal-card compact-modal" role="dialog" aria-modal="true" aria-labelledby="action-dialog-title"><div className="modal-heading"><div><p className="eyebrow">操作确认</p><h2 id="action-dialog-title">{title}</h2><p>{description}</p></div><button className="modal-close" onClick={onCancel} aria-label="关闭对话框">×</button></div>{inputLabel && <label className="modal-field" htmlFor="action-dialog-input">{inputLabel}<input id="action-dialog-input" autoFocus value={inputValue ?? ''} onChange={event => onInputChange?.(event.target.value)} placeholder={inputPlaceholder} /></label>}<div className="modal-actions"><button className="subtle-button" onClick={onCancel}>取消</button><button className={danger ? 'subtle-button danger-action' : 'primary'} disabled={busy || Boolean(inputLabel && !inputValue?.trim())} onClick={onConfirm}>{busy ? '处理中…' : confirmLabel}</button></div></section></div>
}
