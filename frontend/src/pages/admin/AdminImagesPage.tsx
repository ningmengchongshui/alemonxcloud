import { useState } from 'react'
import { ActionDialog } from '@/components/ActionDialog'
import { ImageSourceEditor } from '@/components/ImageSourceEditor'
import {
  useGetAdminCatalogQuery,
  usePullAdminImageVersionMutation,
  useSaveAdminImageMutation
  ,useSaveAdminImageVersionMutation
} from '@/services/cloudApi'
import { Alert, Button, Dialog, PageHeader } from '@/components/ui'
import type { CatalogImage } from '@/types/cloud'

export function AdminImagesPage() {
  const catalog = useGetAdminCatalogQuery()
  const [saveImage] = useSaveAdminImageMutation()
  const [targetID, setTargetID] = useState<string | null>(null)
  const [versionsFor, setVersionsFor] = useState<CatalogImage | null>(null)
  const [tag, setTag] = useState('latest')
  const [digest, setDigest] = useState('')
  const [versionError, setVersionError] = useState('')
  const [saveVersion, { isLoading: savingVersion }] = useSaveAdminImageVersionMutation()
  const [pullVersion, { isLoading: pulling }] = usePullAdminImageVersionMutation()
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
              <th>可售版本</th>
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
                <td>{image.versions.filter(version => version.enabled).map(version => version.tag).join('、') || '暂无'}</td>
                <td>{image.enabled ? '可售' : '已下架'}</td>
                <td>
                  <button
                    className="text-button"
                    onClick={() => setTargetID(image.id)}
                  >
                    {image.enabled ? '下架' : '启用'}
                  </button>
                  <button className="text-button" onClick={() => { setVersionsFor(image); setTag('latest'); setDigest(''); setVersionError('') }}>
                    管理版本
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
      {versionsFor && (
        <Dialog title={`${versionsFor.name} · 版本管理`} description="管理员定义用户可选版本；预拉取会分发至全部启用节点。" onClose={() => setVersionsFor(null)}>
          <div className="space-y-4">
            <div className="space-y-2">
              {versionsFor.versions.map(version => (
                <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-slate-200 p-3 text-sm dark:border-slate-700" key={version.id}>
                  <span><b>{version.tag}</b>{version.imageDigest ? <small className="ml-2 text-slate-500">{version.imageDigest.slice(0, 18)}…</small> : null}</span>
                  <div className="flex gap-2"><span>{version.enabled ? '可售' : '已下架'}</span><Button tone="secondary" loading={pulling} onClick={() => void pullVersion(version)}>预拉取</Button></div>
                </div>
              ))}
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <label>版本标签<input value={tag} onChange={event => setTag(event.target.value)} placeholder="v1.2.0 或 latest" /></label>
              <label>镜像摘要（可选）<input value={digest} onChange={event => setDigest(event.target.value)} placeholder="sha256:..." /></label>
            </div>
            {versionError ? <Alert tone="error">{versionError}</Alert> : null}
            <div className="flex justify-end gap-2"><Button tone="secondary" onClick={() => setVersionsFor(null)}>关闭</Button><Button loading={savingVersion} onClick={() => void saveVersion({ id: '', imageId: versionsFor.id, tag, imageDigest: digest, enabled: true }).unwrap().then(() => { setTag(''); setDigest('') }).catch(error => setVersionError(error?.data?.message ?? '保存版本失败'))}>新增版本</Button></div>
          </div>
        </Dialog>
      )}
    </section>
  )
}
