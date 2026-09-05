import { useState } from 'react'
import { ActionDialog } from '@/components/ActionDialog'
import { PlanEditor } from '@/components/PlanEditor'
import {
  useGetAdminCatalogQuery,
  useSaveAdminPlanMutation
} from '@/services/cloudApi'
import { Button, PageHeader } from '@/components/ui'

export function AdminCatalogPage() {
  const catalog = useGetAdminCatalogQuery()
  const [savePlan] = useSaveAdminPlanMutation()
  const [targetID, setTargetID] = useState<string | null>(null)
  const target = catalog.data?.plans.find(plan => plan.id === targetID)
  async function toggle() {
    if (!target) return
    await savePlan({ ...target, enabled: !target.enabled }).unwrap()
    setTargetID(null)
  }
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="商品管理"
        title="商品目录"
        description="仅管理用户可购买的计算套餐、资源规格和月度价格。镜像来源请前往独立页面维护。"
        actions={
          <Button
            tone="secondary"
            loading={catalog.isFetching}
            onClick={() => void catalog.refetch()}
          >
            ↻ 刷新
          </Button>
        }
      />
      <div className="mb-5 flex justify-end">
        <PlanEditor />
      </div>
      <section className="admin-table-wrap">
        <table>
          <thead>
            <tr>
              <th>套餐</th>
              <th>配置</th>
              <th>月价</th>
              <th>状态</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(catalog.data?.plans ?? []).map(plan => (
              <tr key={plan.id}>
                <td>
                  <b>{plan.name}</b>
                </td>
                <td>
                  {plan.cpu} 核 / {plan.memoryMB / 1024} GB
                </td>
                <td>¥{(plan.monthlyPriceFen / 100).toFixed(2)}</td>
                <td>{plan.enabled ? '可售' : '已下架'}</td>
                <td>
                  <button
                    className="text-button"
                    onClick={() => setTargetID(plan.id)}
                  >
                    {plan.enabled ? '下架' : '启用'}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
      {target && (
        <ActionDialog
          title={`${target.enabled ? '下架' : '启用'}套餐`}
          description={`确定${target.enabled ? '下架' : '启用'} ${target.name} 吗？`}
          confirmLabel="确认操作"
          danger={target.enabled}
          onCancel={() => setTargetID(null)}
          onConfirm={() => void toggle()}
        />
      )}
    </section>
  )
}
