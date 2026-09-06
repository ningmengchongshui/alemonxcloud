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
  | 'benefits'
  | 'benefit-redemptions'
  | 'price-tiers'
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
  containerName?: string
  activeTask?: Pick<Task, 'id' | 'action' | 'status'>
  terminalOnly?: boolean
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
  currentPlanId?: string
  currentPlanName?: string
  planChangeStatus?: 'processing' | 'succeeded' | 'failed' | 'needs_review'
  planChangeId?: string
}
export interface WorkspaceEntry { name: string; path: string; kind: 'file' | 'directory' | 'symlink'; size: number; modifiedAt: string }
export interface WorkspaceListing { path: string; entries: WorkspaceEntry[] }
export interface WorkspaceFile { path: string; content: string; size: number; modifiedAt: string }
export interface PlanChangeQuote {
  quoteId: string
  instanceId: string
  currentPlanId: string
  currentPlanName: string
  targetPlanId: string
  targetPlanName: string
  currentCpu: number
  currentMemoryMB: number
  targetCpu: number
  targetMemoryMB: number
  remainingSeconds: number
  deltaFen: number
  chargeFen: number
  refundFen: number
  expiresAt: string
  summary: string
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
  terminalOnly: boolean
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
  tierDiscounts?: Record<number, number>
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
  benefitSnapshot?: {
    name?: string
    goal?: string
    benefitType?: string
    bonusDays?: number
  }
  bonusDays?: number
  promoCodeMask?: string
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
export interface PriceQuote {
  listAmountFen: number
  discountAmountFen: number
  amountFen: number
  bonusDays: number
  tierMonths?: number
  tierDiscountBps?: number
  quoteSummary: string
  program?: {
    id: string
    name: string
    goal: string
    benefitType: string
    triggerType: string
    codeMask?: string
  }
}
export interface PlanPriceTier {
  id: string
  planId: string
  months: number
  discountBps: number
  enabled: boolean
}
export interface BenefitProgram {
  id: string
  name: string
  goal: 'first_purchase' | 'multi_month' | 'renewal_recovery' | 'channel'
  status: 'draft' | 'scheduled' | 'active' | 'paused' | 'ended'
  triggerType: 'automatic' | 'promo_code' | 'targeted'
  orderScope: 'purchase' | 'renewal' | 'both'
  benefitType: 'fixed_discount' | 'percent_discount' | 'bonus_days'
  benefitValue: number
  minAmountFen: number
  planIds: string[]
  monthValues: number[]
  audienceType: string
  startsAt?: string
  endsAt?: string
  perUserLimit: number
  totalLimit: number
  usedCount: number
  cashBudgetFen: number
  cashSpentFen: number
  grantDaysLimit: number
  grantDaysUsed: number
  priority: number
  channelLabel?: string
  code?: string
  codeMask?: string
  codeTotalLimit?: number
  codePerUserLimit?: number
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
  diskTotalBytes?: number
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
  heartbeatAt?: string
  recoveryCount?: number
}
export interface TaskEvent {
  id: number
  taskId: string
  event: string
  detail: string
  createdAt: string
}
export interface InstanceTaskRecord {
  task: Task
  events: TaskEvent[]
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
  leaseRecoveryTasks24h: number
  tasksNeedsReview?: number
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
