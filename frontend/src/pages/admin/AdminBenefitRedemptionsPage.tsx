import {
  DataTable,
  EmptyState,
  LoadingState,
  PageHeader,
  StatusBadge
} from '@/components/ui'
import {
  useGetAdminBenefitProgramsQuery,
  useGetAdminBenefitRedemptionsQuery
} from '@/services/cloudApi'

const money = (value: number) => `¥${(value / 100).toFixed(2)}`

export function AdminBenefitRedemptionsPage() {
  const { data: items = [], isLoading } = useGetAdminBenefitRedemptionsQuery()
  const { data: programs = [] } = useGetAdminBenefitProgramsQuery()
  const nameFor = (id: string) =>
    programs.find(item => item.id === id)?.name || '已删除的权益方案'

  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="营销运营"
        title="权益核销记录"
        description="查看已生效权益对应的订单、优惠金额与赠送天数。"
      />
      {isLoading ? (
        <LoadingState>正在加载核销记录…</LoadingState>
      ) : items.length === 0 ? (
        <EmptyState
          title="暂无核销记录"
          description="用户完成购买或续费并命中权益后，记录会显示在这里。"
        />
      ) : (
        <DataTable
          title="核销明细"
          description={`共 ${items.length} 条记录，按最新核销时间排序。`}
        >
          <thead className="bg-slate-50 text-[10px] font-extrabold tracking-wide text-slate-500 dark:bg-slate-900 dark:text-slate-300">
            <tr>
              <th className="px-5 py-3">权益方案</th>
              <th className="px-5 py-3">用户 / 订单</th>
              <th className="px-5 py-3">权益内容</th>
              <th className="px-5 py-3">核销时间</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-700">
            {items.map(item => (
              <tr
                key={item.id}
                className="text-xs text-slate-600 dark:text-slate-200"
              >
                <td className="px-5 py-4 font-bold text-slate-800 dark:text-white">
                  {nameFor(item.programId)}
                </td>
                <td className="px-5 py-4">
                  <div>{item.ownerId}</div>
                  <small className="mt-1 block text-[10px] text-slate-400">
                    {item.orderId}
                  </small>
                </td>
                <td className="px-5 py-4">
                  <StatusBadge tone="success">
                    {item.discountAmountFen
                      ? `立减 ${money(item.discountAmountFen)}`
                      : `赠送 ${item.bonusDays} 天`}
                  </StatusBadge>
                </td>
                <td className="whitespace-nowrap px-5 py-4 text-slate-500 dark:text-slate-300">
                  {new Date(item.createdAt).toLocaleString()}
                </td>
              </tr>
            ))}
          </tbody>
        </DataTable>
      )}
    </section>
  )
}
