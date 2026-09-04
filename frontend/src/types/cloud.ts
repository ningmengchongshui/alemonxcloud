export type Page = 'instances' | 'create' | 'admin'

export interface CurrentUser {
  username: string
  isAdmin: boolean
}

export interface Instance {
  id: string
  name: string
  image: string
  version: string
  spec: string
  status: string
  ip: string
  createdAt: string
}

export interface CreateInstanceInput {
  name: string
  image: string
  version: string
  spec: string
  cpu: number
  memoryMB: number
}
