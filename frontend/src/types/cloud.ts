export type Page =
  | 'overview'
  | 'instances'
  | 'create'
  | 'orders'
  | 'wallet'
  | 'offers'
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
  | 'promotions'
  | 'coupons'
  | 'redemptions'
  | 'settings'

export interface CurrentUser {
  id?: string
  username: string
  isAdmin: boolean
  email?: string
  phone?: string
  avatar?: string
}

export interface RechargeContact {
  name: string
  url: string
}

export interface Instance {
  id: string
  name: string
  image: string
  version: string
  spec: string
  status: string
  runtimeStatus?: string
  bandwidthMbps?: number
  destroyAt?: string
  destroyedAt?: string
  purgeAt?: string
  destroyReason?: 'refund' | 'expired' | 'manual' | 'legacy'
  archivedAt?: string
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
  versions: ImageVersion[]
}
export interface PublicCatalogImage {
  id: string
  name: string
  versions: Array<{ tag: string }>
}
export interface ImageVersion {
  id: string
  imageId: string
  tag: string
  imageDigest: string
  enabled: boolean
  status: 'draft' | 'syncing' | 'ready' | 'failed' | 'disabled'
  lastError?: string
  publishedAt?: string
  createdAt?: string
}
export interface Plan {
  id: string
  name: string
  cpu: number
  memoryMB: number
  bandwidthMbps: number
  monthlyPriceFen: number
  enabled: boolean
  sortOrder: number
}
export interface Catalog {
  images: PublicCatalogImage[]
  plans: Plan[]
}
export interface AdminCatalog {
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
  listAmountFen: number
  discountAmountFen: number
  promotionSnapshot?: { name?: string; kind?: string; couponMask?: string }
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
export interface PriceCandidate {
  id: string
  kind: 'newcomer' | 'first_plan_purchase' | 'campaign' | 'coupon'
  name: string
  label: string
  eligibilityReason?: string
  discountAmountFen: number
  payableAmountFen: number
  isDefault: boolean
}
export interface PriceQuote {
  listAmountFen: number
  discountAmountFen: number
  amountFen: number
  selectedId?: string
  payFullPrice?: boolean
  candidates: PriceCandidate[]
}
export interface Promotion {
  id: string
  name: string
  kind: 'newcomer' | 'first_plan_purchase' | 'campaign'
  scope: 'purchase' | 'renewal' | 'both'
  discountType: 'fixed' | 'percent'
  discountValue: number
  minAmountFen: number
  maxDiscountFen: number
  planIDs: string[]
  imageIDs: string[]
  monthValues: string[]
  startsAt?: string
  endsAt?: string
  totalLimit: number
  perUserLimit: number
  usedCount: number
  enabled: boolean
  createdAt: string
}
export interface Coupon {
  id: string
  promotionId: string
  codeMask: string
  mode: 'single' | 'general'
  enabled: boolean
  totalLimit: number
  perUserLimit: number
  usedCount: number
  createdAt: string
}
export interface CouponRedemption {
  id: string
  promotionId: string
  couponId?: string
  ownerId: string
  orderId: string
  discountAmountFen: number
  createdAt: string
}
export interface CouponBatch {
  id: string
  name: string
  status: 'active' | 'paused'
  distributionMode: 'public' | 'targeted'
  discountType: 'fixed' | 'percent'
  discountValue: number
  minAmountFen: number
  maxDiscountFen: number
  scope: 'purchase' | 'renewal' | 'both'
  planIDs: string[]
  imageIDs: string[]
  monthValues: string[]
  startsAt?: string
  endsAt?: string
  issueLimit: number
  perUserLimit: number
  issuedCount: number
  createdAt: string
}
export interface UserCoupon {
  id: string
  batchId: string
  ownerId: string
  status: 'available' | 'used' | 'expired' | 'voided'
  issueSource: 'public' | 'targeted' | 'legacy'
  expiresAt?: string
  usedAt?: string
  createdAt: string
  batch?: CouponBatch
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
  agentVersion?: string
  agentApiVersion?: number
  agentCapabilities?: string[]
  agentCompatibility?: 'compatible' | 'legacy' | 'outdated'
  offlineInstanceCount?: number
  pendingCleanupTasks?: number
  lastAgentError?: string
}
export interface Task {
  id: string
  instanceId: string
  action: string
  status: string
  attempts: number
  lastError: string
  createdAt: string
  claimedAt?: string
  claimExpiresAt?: string
  workerId?: string
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
  deploymentFailed: number
  runtimeMissing: number
  destroyBlocked: number
  offlineInstances: number
  leaseRecoveries24h: number
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
