import { ReactNode } from "react"

export function Field({
  label,
  children
}: {
  label?: string
  children: ReactNode
}) {
  return (
    <label className="block text-[11px] font-bold text-slate-700 dark:text-slate-100">
      {label}
      {children}
    </label>
  )
}
