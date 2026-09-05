import { useState } from 'react'
import { ActionDialog } from '@/components/ActionDialog'
import { ImageSourceEditor } from '@/components/ImageSourceEditor'
import {
  useGetAdminCatalogQuery,
  useDeleteAdminImageVersionMutation,
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
  const [renaming, setRenaming] = useState<CatalogImage | null>(null)
  const [softwareName, setSoftwareName] = useState('')
  const [versionsFor, setVersionsFor] = useState<CatalogImage | null>(null)
  const [tag, setTag] = useState('latest')
  const [versionError, setVersionError] = useState('')
  const [saveVersion, { isLoading: savingVersion }] =
    useSaveAdminImageVersionMutation()
  const [pullVersion, { isLoading: pulling }] =
    usePullAdminImageVersionMutation()
  const [deleteVersion, { isLoading: deletingVersion }] =
    useDeleteAdminImageVersionMutation()
  const target = catalog.data?.images.find(image => image.id === targetID)
  const currentVersions = versionsFor
    ? (catalog.data?.images.find(image => image.id === versionsFor.id)
        ?.versions ?? versionsFor.versions)
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
        title="软件与版本"
        description="配置可信软件和可购买版本；用户只会看到软件名称与版本号。"
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
              <th>软件名称</th>
              <th>镜像仓库</th>
              <th>可购买版本</th>
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
                  {(image.versions ?? [])
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
                    {image.enabled ? '下架软件' : '启用软件'}
                  </button>
                  <button
                    className="text-button"
                    onClick={() => {
                      setRenaming(image)
                      setSoftwareName(image.name)
                    }}
                  >
                    修改名称
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
          title={`${target.enabled ? '下架' : '启用'}软件`}
          description={`确定${target.enabled ? '下架' : '启用'}软件 ${target.name} 吗？`}
          confirmLabel="确认操作"
          danger={target.enabled}
          onCancel={() => setTargetID(null)}
          onConfirm={() => void toggle()}
        />
      )}
      {renaming && (
        <Dialog
          title="修改软件名称"
          description="仅修改用户和运营端显示名称，不会改变镜像仓库或已部署实例。"
          onClose={() => setRenaming(null)}
        >
          <div className="space-y-4">
            <label>
              软件名称
              <input
                value={softwareName}
                onChange={event => setSoftwareName(event.target.value)}
                data-autofocus
              />
            </label>
            <div className="flex justify-end gap-2">
              <Button tone="secondary" onClick={() => setRenaming(null)}>
                取消
              </Button>
              <Button
                disabled={!softwareName.trim()}
                onClick={() =>
                  void saveImage({ ...renaming, name: softwareName.trim() })
                    .unwrap()
                    .then(() => setRenaming(null))
                    .catch(() => setVersionError('软件名称保存失败'))
                }
              >
                保存名称
              </Button>
            </div>
          </div>
        </Dialog>
      )}
      {versionsFor && (
        <Dialog
          title={`${versionsFor.name} · 版本管理`}
          description="新增时填写版本号并保存；需要拉取最新镜像时，点击对应版本的“更新”。"
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
                  </span>
                  <div className="flex gap-2">
                    {version.enabled || version.status === 'ready' ? (
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
                      更新
                    </Button>
                    <Button
                      tone="danger"
                      loading={deletingVersion}
                      onClick={() =>
                        void deleteVersion({
                          imageId: version.imageId,
                          versionId: version.id
                        })
                      }
                    >
                      删除
                    </Button>
                  </div>
                </div>
              ))}
            </div>
            <div className="grid gap-3">
              <label>
                版本号
                <input
                  value={tag}
                  onChange={event => setTag(event.target.value)}
                  placeholder="例如 v2.4.1"
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
                      setVersionError(
                        typeof error?.data?.message === 'string'
                          ? error.data.message
                          : '版本保存失败'
                      )
                    )
                }
              >
                保存版本
              </Button>
            </div>
          </div>
        </Dialog>
      )}
    </section>
  )
}
