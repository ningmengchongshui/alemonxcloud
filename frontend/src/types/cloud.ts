export type Page =
  | 'overview'
  | 'instances'
  | 'create'
  | 'orders'
  | 'wallet'
  | 'notifications'
  | 'tickets'
export type SuperPage =
  | 'overview'
  | 'catalog'
  | 'images'
  | 'nodes'
  | 'orders'
  | 'tasks'
  | 'users'
  | 'tickets'
  | 'audit'

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

export interface CatalogImage {
  id: string
  name: string
  imageRef: string
  imageDigest: string
  version: string
  enabled: boolean
}
export interface Plan {
  id: string
  name: string
  cpu: number
  memoryMB: number
  monthlyPriceFen: number
  enabled: boolean
  sortOrder: number
}
export interface Catalog {
  images: CatalogImage[]
  plans: Plan[]
}
export interface Order {
  id: string
  ownerId: string
  planId: string
  imageId: string
  instanceId: string
  amountFen: number
  status: string
  paymentNote: string
  serviceStartsAt?: string
  expiresAt?: string
  refundedAt?: string
  refundAmountFen?: number
  refundWalletEntryId?: string
  createdAt: string
  planName: string
  imageName: string
  imageVersion: string
}
export interface Node {
  id: string
  name: string
  agentURL: string
  agentToken?: string
  cpuTotal: number
  memoryTotalMB: number
  cpuDetected: number
  memoryDetectedMB: number
  cpuReserved: number
  memoryReservedMB: number
  enabled: boolean
  lastHeartbeatAt?: string
  dockerVersion?: string
  diskAvailableBytes?: number
  managedContainerCount?: number
}
export interface Task {
  id: string
  instanceId: string
  action: string
  status: string
  attempts: number
  lastError: string
  createdAt: string
}
export interface Wallet {
  id: string
  username: string
  email: string
  balanceFen: number
  lastLoginAt: string
}
export interface WalletEntry {
  id: string
  amountFen: number
  balanceAfterFen: number
  type: string
  note: string
  actorId?: string
  orderId?: string
  createdAt: string
}
export interface RefundQuote {
  orderId: string
  eligible: boolean
  reason?: string
  totalDays: number
  remainingDays: number
  prepaidDays: number
  refundableDays: number
  refundAmountFen: number
  serviceEndsAt: string
  dataPurgeAt: string
}
export interface Notification {
  id: string
  type: string
  title: string
  body: string
  data?: { ticketId?: string }
  readAt?: string
  createdAt: string
}
export interface CloudUser {
  id: string
  username: string
  email: string
  balanceFen: number
  lastLoginAt: string
}
export interface AdminMetrics {
  nodes: Node[]
  taskFailures: number
  taskBacklog: number
  openTickets: number
  urgentTickets: number
}
export type TicketStatus = 'open' | 'in_progress' | 'closed'
export type TicketPriority = 'normal' | 'high' | 'urgent'
export type TicketCategory = 'instance' | 'billing' | 'account' | 'other'
export interface Ticket {
  id: string
  ownerId: string
  category: TicketCategory
  priority: TicketPriority
  subject: string
  instanceId?: string
  orderId?: string
  status: TicketStatus
  lastAdminId?: string
  closedAt?: string
  createdAt: string
  updatedAt: string
}
export interface TicketMessage {
  id: string
  ticketId: string
  senderId: string
  senderRole: 'user' | 'admin'
  body: string
  createdAt: string
}
export interface TicketDetail {
  ticket: Ticket
  messages: TicketMessage[]
}
export interface AuditLog {
  id: number
  actorId: string
  action: string
  targetType: string
  targetId: string
  createdAt: string
}
