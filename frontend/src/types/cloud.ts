export type Page = 'instances' | 'create' | 'orders'
export type SuperPage = 'overview' | 'catalog' | 'nodes' | 'orders' | 'tasks' | 'users' | 'audit'

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

export interface CatalogImage { id: string; name: string; imageRef: string; imageDigest: string; version: string; enabled: boolean }
export interface Plan { id: string; name: string; cpu: number; memoryMB: number; monthlyPriceFen: number; enabled: boolean; sortOrder: number }
export interface Catalog { images: CatalogImage[]; plans: Plan[] }
export interface Order { id: string; ownerId: string; planId: string; imageId: string; instanceId: string; amountFen: number; status: string; paymentNote: string; expiresAt?: string; createdAt: string; planName: string; imageName: string; imageVersion: string }
export interface Node { id: string; name: string; agentURL: string; agentToken?: string; cpuTotal: number; memoryTotalMB: number; cpuReserved: number; memoryReservedMB: number; enabled: boolean; lastHeartbeatAt?: string; dockerVersion?: string; diskAvailableBytes?: number; managedContainerCount?: number }
export interface Task { id: string; instanceId: string; action: string; status: string; attempts: number; lastError: string; createdAt: string }
export interface Wallet { id: string; username: string; email: string; balanceFen: number; lastLoginAt: string }
export interface WalletEntry { id: string; amountFen: number; balanceAfterFen: number; type: string; note: string; createdAt: string }
export interface Notification { id: string; type: string; title: string; body: string; readAt?: string; createdAt: string }
export interface CloudUser { id: string; username: string; email: string; balanceFen: number; lastLoginAt: string }
export interface AdminMetrics { nodes: Node[]; taskFailures: number; taskBacklog: number }
export interface AuditLog { id: number; actorId: string; action: string; targetType: string; targetId: string; createdAt: string }
