import {
  useGetNotificationsQuery,
  useReadAllNotificationsMutation,
  useReadNotificationMutation
} from '@/services/cloudApi'
import { Button, EmptyState, LoadingState, PageHeader } from '@/components/ui'

export function NotificationsPage({
  onOpenTicket
}: {
  onOpenTicket?: (id: string) => void
}) {
  const { data: notifications = [], isLoading } = useGetNotificationsQuery(
    undefined,
    { pollingInterval: 30000 }
  )
  const [read] = useReadNotificationMutation()
  const [readAll, { isLoading: markingAll }] = useReadAllNotificationsMutation()
  const unread = notifications.filter(item => !item.readAt).length
  return (
    <section className="page me-page">
      <PageHeader
        title="通知"
        description="购买、部署、到期和钱包变动都会在这里留下记录。"
        actions={
          unread > 0 ? (
            <Button
              tone="secondary"
              loading={markingAll}
              onClick={() => void readAll()}
            >
              全部标为已读
            </Button>
          ) : undefined
        }
      />
      {isLoading ? (
        <LoadingState>正在加载站内通知…</LoadingState>
      ) : notifications.length === 0 ? (
        <EmptyState
          title="暂无通知"
          description="服务、订单或钱包发生变化时会在这里通知你。"
        />
      ) : (
        <div className="divide-y divide-slate-100 overflow-hidden rounded-xl border border-slate-200 bg-white dark:divide-slate-700 dark:border-slate-700 dark:bg-slate-800">
          {notifications.map(item => (
            <article
              className={`px-5 py-4 ${item.readAt ? '' : 'bg-blue-50/60 dark:bg-blue-950/30'}`}
              key={item.id}
              onClick={() => {
                if (item.data?.ticketId) {
                  void read(item.id)
                  onOpenTicket?.(item.data.ticketId)
                }
              }}
            >
              <div className="flex items-start justify-between gap-4">
                <div>
                  <div className="flex items-center gap-2">
                    <b className="text-sm">{item.title}</b>
                    {!item.readAt && (
                      <span className="rounded-full bg-blue-600 px-2 py-0.5 text-[10px] font-bold text-white">
                        未读
                      </span>
                    )}
                  </div>
                  <p className="mt-2 text-xs leading-5 text-slate-600 dark:text-slate-200">
                    {item.body}
                  </p>
                  <small className="mt-2 block text-[11px] text-slate-400">
                    {new Date(item.createdAt).toLocaleString('zh-CN')}
                  </small>
                </div>
                {!item.readAt && (
                  <button
                    type="button"
                    className="shrink-0 text-xs font-bold text-blue-700 hover:underline dark:text-blue-200"
                    onClick={() => void read(item.id)}
                  >
                    标为已读
                  </button>
                )}
              </div>
            </article>
          ))}
        </div>
      )}
    </section>
  )
}
