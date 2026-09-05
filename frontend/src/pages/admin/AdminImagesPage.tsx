import { useState } from 'react'
import { ActionDialog } from '@/components/ActionDialog'
import { ImageSourceEditor } from '@/components/ImageSourceEditor'
import {
  useGetAdminCatalogQuery,
  usePullAdminImageVersionMutation,
  useSaveAdminImageMutation,
  useSaveAdminImageVersionMutation
} from '@/services/cloudApi'
import { Alert, Button, Dialog, PageHeader } from '@/components/ui'
import type { CatalogImage } from '@/types/cloud'

export function AdminImagesPage() {
  const catalog = useGetAdminCatalogQuery()
  const [saveImage] = useSaveAdminImageMutation()
  const [targetID, setTargetID] = useState<string | null>(null)
  const [versionsFor, setVersionsFor] = useState<CatalogImage | null>(null)
  const [tag, setTag] = useState('latest')
  const [versionError, setVersionError] = useState('')
  const [saveVersion, { isLoading: savingVersion }] =
    useSaveAdminImageVersionMutation()
  const [pullVersion, { isLoading: pulling }] =
    usePullAdminImageVersionMutation()
  const target = catalog.data?.images.find(image => image.id === targetID)
  const currentVersions = versionsFor
    ? catalog.data?.images.find(image => image.id === versionsFor.id)?.versions ?? versionsFor.versions
    : []
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
                <td>
                  {image.versions
                    .filter(version => version.enabled)
                    .map(version => version.tag)
                    .join('、') || '暂无'}
                </td>
                <td>{image.enabled ? '可售' : '已下架'}</td>
                <td>
                  <button
                    className="text-button"
                    onClick={() => setTargetID(image.id)}
                  >
                    {image.enabled ? '下架' : '启用'}
                  </button>
                  <button
                    className="text-button"
                    onClick={() => {
                      setVersionsFor(image)
                      setTag('latest')
                      setVersionError('')
                    }}
                  >
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
        <Dialog
          title={`${versionsFor.name} · 版本管理`}
          description="版本先作为草稿同步至健康节点；只有摘要一致的版本才会发布给用户。"
          onClose={() => setVersionsFor(null)}
        >
          <div className="space-y-4">
            <div className="space-y-2">
              {currentVersions.map(version => (
                <div
                  className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-slate-200 p-3 text-sm dark:border-slate-700"
                  key={version.id}
                >
                  <span>
                    <b>{version.tag}</b>
                    {version.imageDigest ? (
                      <small className="ml-2 text-slate-500">
                        {version.imageDigest.slice(0, 18)}…
                      </small>
                    ) : null}
                  </span>
                  <div className="flex gap-2">
                    <span>
                      {version.status === 'ready'
                        ? version.enabled
                          ? '已发布'
                          : '已下架'
                        : version.status === 'syncing'
                          ? '同步中'
                          : version.status === 'failed'
                            ? '校验失败'
                            : '草稿'}
                    </span>
                    {version.status === 'ready' ? (
                      <Button
                        tone="secondary"
                        loading={savingVersion}
                        onClick={() =>
                          void saveVersion({
                            ...version,
                            enabled: !version.enabled
                          })
                        }
                      >
                        {version.enabled ? '下架' : '上架'}
                      </Button>
                    ) : null}
                    <Button
                      tone="secondary"
                      loading={pulling}
                      disabled={version.status === 'syncing'}
                      onClick={() => void pullVersion(version)}
                    >
                      同步并发布
                    </Button>
                  </div>
                  {version.lastError ? (
                    <p className="m-0 w-full text-xs text-rose-600 dark:text-rose-300">
                      {version.lastError}
                    </p>
                  ) : null}
                </div>
              ))}
            </div>
            <div className="grid gap-3">
              <label>
                版本标签
                <input
                  value={tag}
                  onChange={event => setTag(event.target.value)}
                  placeholder="v1.2.0 或 latest"
                />
              </label>
            </div>
            {versionError ? <Alert tone="error">{versionError}</Alert> : null}
            <div className="flex justify-end gap-2">
              <Button tone="secondary" onClick={() => setVersionsFor(null)}>
                关闭
              </Button>
              <Button
                loading={savingVersion}
                onClick={() =>
                  void saveVersion({
                    id: '',
                    imageId: versionsFor.id,
                    tag,
                    imageDigest: '',
                    enabled: false,
                    status: 'draft'
                  })
                    .unwrap()
                    .then(() => {
                      setTag('')
                    })
                    .catch(error =>
                      setVersionError(error?.data?.message ?? '保存版本失败')
                    )
                }
              >
                新增草稿版本
              </Button>
            </div>
          </div>
        </Dialog>
      )}
    </section>
  )
}
