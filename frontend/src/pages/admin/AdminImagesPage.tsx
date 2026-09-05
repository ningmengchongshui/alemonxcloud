import { useState } from 'react'
import { ActionDialog } from '@/components/ActionDialog'
import { ImageSourceEditor } from '@/components/ImageSourceEditor'
import {
  useGetAdminCatalogQuery,
  useSaveAdminImageMutation
} from '@/services/cloudApi'
import { Button, PageHeader } from '@/components/ui'

export function AdminImagesPage() {
  const catalog = useGetAdminCatalogQuery()
  const [saveImage] = useSaveAdminImageMutation()
  const [targetID, setTargetID] = useState<string | null>(null)
  const target = catalog.data?.images.find(image => image.id === targetID)
  async function toggle() {
    if (!target) return
    await saveImage({ ...target, enabled: !target.enabled }).unwrap()
    setTargetID(null)
  }
  return (
    <section className="page super-page">
      <PageHeader
        eyebrow="镜像管理"
        title="镜像来源"
        description="只登记可信镜像地址和可用版本；用户仅能从这里配置的来源创建服务。"
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
        <ImageSourceEditor />
      </div>
      <section className="admin-table-wrap">
        <table>
          <thead>
            <tr>
              <th>镜像名称</th>
              <th>镜像地址</th>
              <th>默认版本</th>
              <th>状态</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(catalog.data?.images ?? []).map(image => (
              <tr key={image.id}>
                <td>
                  <b>{image.name}</b>
                </td>
                <td>
                  <code>{image.imageRef}</code>
                </td>
                <td>{image.version || 'latest'}</td>
                <td>{image.enabled ? '可售' : '已下架'}</td>
                <td>
                  <button
                    className="text-button"
                    onClick={() => setTargetID(image.id)}
                  >
                    {image.enabled ? '下架' : '启用'}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
      {target && (
        <ActionDialog
          title={`${target.enabled ? '下架' : '启用'}镜像来源`}
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
