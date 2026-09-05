import { AdminCatalogPage } from '@/pages/admin/AdminCatalogPage'
import { AdminImagesPage } from '@/pages/admin/AdminImagesPage'
import { AdminNodesPage } from '@/pages/admin/AdminNodesPage'
import { AdminOverviewPage } from '@/pages/admin/AdminOverviewPage'
import { AdminTicketsPage } from '@/pages/admin/AdminTicketsPage'
import { AdminPromotionsPage } from '@/pages/admin/AdminPromotionsPage'
import {
  AdminCouponRedemptionsPage,
  AdminCouponsPage
} from '@/pages/admin/AdminPromotionOperations'
import { AdminSettingsPage } from '@/pages/admin/AdminSettingsPage'
import {
  AdminAuditPage,
  AdminOrdersPage,
  AdminTasksPage,
  AdminUsersPage
} from '@/pages/admin/AdminRecordsPages'
import type { SuperPage } from '@/types/cloud'

// Route-level dispatcher: each page owns its own query and mutation lifecycle.
export function AdminPage({
  page,
  onOpenWalletHistory,
  onCreatePromotion,
  onEditPromotion
}: {
  page: SuperPage
  onOpenWalletHistory?: (user: { id: string }) => void
  onCreatePromotion?: () => void
  onEditPromotion?: (id: string) => void
}) {
  if (page === 'catalog') return <AdminCatalogPage />
  if (page === 'images') return <AdminImagesPage />
  if (page === 'nodes') return <AdminNodesPage />
  if (page === 'orders') return <AdminOrdersPage />
  if (page === 'tasks') return <AdminTasksPage />
  if (page === 'tickets') return <AdminTicketsPage />
  if (page === 'coupons')
    return <AdminCouponsPage onBack={() => window.history.back()} />
  if (page === 'redemptions')
    return <AdminCouponRedemptionsPage onBack={() => window.history.back()} />
  if (page === 'promotions')
    return (
      <AdminPromotionsPage
        onCreate={onCreatePromotion}
        onEdit={onEditPromotion}
      />
    )
  if (page === 'settings') return <AdminSettingsPage />
  if (page === 'users')
    return <AdminUsersPage onOpenWalletHistory={onOpenWalletHistory} />
  if (page === 'audit') return <AdminAuditPage />
  return <AdminOverviewPage />
}
