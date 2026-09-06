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
import {
  Alert,
  Button,
  Dialog,
  DialogFooter,
  dialogFieldClass,
  dialogLabelClass,
  PageHeader
} from '@/components/ui'
import type { CatalogImage } from '@/types/cloud'

export function AdminImagesPage() {
  const catalog = useGetAdminCatalogQuery()
  const [saveImage] = useSaveAdminImageMutation()
  const [targetID, setTargetID] = useState<string | null>(null)
  const [renaming, setRenaming] = useState<CatalogImage | null>(null)
  const [softwareName, setSoftwareName] = useState('')
  const [webSupported, setWebSupported] = useState(false)
  const [versionsFor, setVersionsFor] = useState<CatalogImage | null>(null)
  const [tag, setTag] = useState('')
  const [addingVersion, setAddingVersion] = useState(false)
  const [versionError, setVersionError] = useState('')
  const [saveVersion, { isLoading: savingVersion }] =
    useSaveAdminImageVersionMutation()
  const [pullVersion, { isLoading: pulling }] =
    usePullAdminImageVersionMutation()
  const [deleteVersion, { isLoading: deletingVersion }] =
    useDeleteAdminImageVersionMutation()
  // Older catalog records can lack the optional versions array. Treat that as
  // an empty list so opening version management never crashes the console.
  const images = Array.isArray(catalog.data?.images) ? catalog.data.images : []
  const target = images.find(image => image.id === targetID)
  const currentVersions = versionsFor
    ? (images.find(image => image.id === versionsFor.id)?.versions ??
      versionsFor.versions ?? [])
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
        description="配置可信软件、可购买版本与访问能力；镜像默认使用终端，可按需启用 Web 服务。"
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
            {images.map(image => (
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
                <td><div>{image.enabled ? '可售' : '已下架'}</div>{image.terminalOnly === false && <small className="text-blue-700">支持 Web</small>}</td>
                <td className="flex gap-2">
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
                      setWebSupported(image.terminalOnly === false)
                    }}
                  >
                    软件设置
                  </button>
                  <button
                    className="text-button"
                    onClick={() => {
                      setVersionsFor(image)
                      setTag('')
                      setAddingVersion(false)
                      setVersionError('')
                    }}
                  >
                    版本配置
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
          title="软件设置"
          description="修改显示名称和访问方式，不会改变镜像仓库或已部署实例。"
          onClose={() => setRenaming(null)}
        >
          <form
            className="space-y-4"
            onSubmit={event => {
              event.preventDefault()
              if (!softwareName.trim()) return
              void saveImage({ ...renaming, name: softwareName.trim(), terminalOnly: !webSupported })
                .unwrap()
                .then(() => setRenaming(null))
                .catch(() => setVersionError('软件名称保存失败'))
            }}
          >
            <label className={dialogLabelClass} htmlFor="software-display-name">
              软件名称
              <input
                id="software-display-name"
                className={dialogFieldClass}
                value={softwareName}
                onChange={event => setSoftwareName(event.target.value)}
                placeholder="请输入软件显示名称"
                data-autofocus
              />
            </label>
            <label className="flex cursor-pointer items-start gap-3 rounded-lg border border-slate-200 p-3 text-xs text-slate-700 dark:border-slate-700 dark:text-slate-100">
              <input className="mt-0.5" type="checkbox" checked={webSupported} onChange={event => setWebSupported(event.target.checked)} />
              <span><b className="block">支持 Web 服务</b><small className="mt-1 block leading-5 text-slate-500 dark:text-slate-300">默认关闭。开启后，用户实例页会额外显示“Web 服务”入口；终端入口始终可用。</small></span>
            </label>
            {versionError && <Alert tone="error">{versionError}</Alert>}
            <DialogFooter>
              <Button
                type="button"
                tone="secondary"
                onClick={() => setRenaming(null)}
              >
                取消
              </Button>
              <Button type="submit" disabled={!softwareName.trim()}>
                保存名称
              </Button>
            </DialogFooter>
          </form>
        </Dialog>
      )}
      {versionsFor && (
        <Dialog
          title={`${versionsFor.name} · 版本管理`}
          description="这里配置用户购买时可选择的版本。"
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
              {currentVersions.length === 0 && (
                <p className="m-0 rounded-lg bg-slate-50 p-3 text-sm text-slate-500 dark:bg-slate-900 dark:text-slate-300">
                  还没有版本，点击下方“＋ 新增版本”添加。
                </p>
              )}
            </div>
            {addingVersion ? (
              <div className="flex flex-wrap items-end gap-2 rounded-lg border border-slate-200 p-3 dark:border-slate-700">
                <label className="min-w-52 flex-1">
                  版本号
                  <input
                    value={tag}
                    onChange={event => setTag(event.target.value)}
                    placeholder="例如 v2.4.1"
                    data-autofocus
                  />
                </label>
                <Button
                  tone="secondary"
                  onClick={() => {
                    setAddingVersion(false)
                    setTag('')
                  }}
                >
                  取消
                </Button>
                <Button
                  loading={savingVersion}
                  disabled={!tag.trim()}
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
                        setAddingVersion(false)
                      })
                      .catch(() => setVersionError('版本保存失败'))
                  }
                >
                  保存
                </Button>
              </div>
            ) : (
              <Button tone="secondary" onClick={() => setAddingVersion(true)}>
                ＋ 新增版本
              </Button>
            )}
            {versionError ? <Alert tone="error">{versionError}</Alert> : null}
            <div className="flex justify-end gap-2">
              <Button tone="secondary" onClick={() => setVersionsFor(null)}>
                关闭
              </Button>
            </div>
          </div>
        </Dialog>
      )}
    </section>
  )
}
