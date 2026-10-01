import type { Instance, Task } from '@/types/cloud'

export interface InstanceHostAccessProps {
  instance: Instance
  busy: boolean
  onSubmitted: (task: Task) => void
}
