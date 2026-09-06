export type ToastTone = 'error' | 'success' | 'info'

export type ToastMessage = {
  id: number
  tone: ToastTone
  title: string
  detail?: string
}

type Listener = (toast: ToastMessage) => void

const listeners = new Set<Listener>()
let nextID = 1

export const toast = {
  show(tone: ToastTone, title: string, detail?: string) {
    const item = { id: nextID++, tone, title, detail }
    listeners.forEach(listener => listener(item))
  },
  error(title: string, detail?: string) {
    this.show('error', title, detail)
  },
  success(title: string, detail?: string) {
    this.show('success', title, detail)
  },
  info(title: string, detail?: string) {
    this.show('info', title, detail)
  },
  subscribe(listener: Listener) {
    listeners.add(listener)
    return () => { listeners.delete(listener) }
  }
}
