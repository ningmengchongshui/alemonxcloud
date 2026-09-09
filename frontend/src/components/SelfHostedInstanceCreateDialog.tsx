import { useEffect, useState } from 'react'
import {
  Alert,
  Button,
  Dialog,
  DialogFooter,
  dialogFieldClass,
  dialogLabelClass
} from '@/components/ui'
import {
  useCreateSelfHostedNodeInstanceMutation,
  useGetCatalogQuery
} from '@/services/cloudApi'
import type { SelfHostedNode } from '@/types/cloud'

export function SelfHostedInstanceCreateDialog({
  node,
  onClose
}: {
  node: SelfHostedNode
  onClose: () => void
}) {
  const { data: catalog, isLoading: catalogLoading } = useGetCatalogQuery()
  const [createInstance, { isLoading: submitting }] =
    useCreateSelfHostedNodeInstanceMutation()
  const [name, setName] = useState('')
  const [imageID, setImageID] = useState('')
  const [imageVersion, setImageVersion] = useState('')
  const [cpu, setCPU] = useState('1')
  const [memoryMB, setMemoryMB] = useState('1024')
  const [error, setError] = useState('')

  const selectedImage = catalog?.images.find(image => image.id === imageID)
  const selectedVersions = selectedImage?.versions ?? []

  useEffect(() => {
    if (imageID || !catalog?.images.length) return
    const image = catalog.images[0]
    const version =
      image.versions.find(item => item.tag.toLowerCase() === 'latest') ??
      image.versions[0]
    setImageID(image.id)
    setImageVersion(version?.tag ?? '')
  }, [catalog, imageID])

  function chooseImage(nextImageID: string) {
    const image = catalog?.images.find(item => item.id === nextImageID)
    const version =
      image?.versions.find(item => item.tag.toLowerCase() === 'latest') ??
      image?.versions[0]
    setImageID(nextImageID)
    setImageVersion(version?.tag ?? '')
    setError('')
  }

  function submit() {
    const cpuValue = Number(cpu)
    const memoryValue = Number(memoryMB)
    if (!name.trim() || !imageID || !imageVersion) {
      setError('请填写实例名称，并选择镜像和版本。')
      return
    }
    if (!Number.isFinite(cpuValue) || cpuValue <= 0 || !Number.isInteger(memoryValue) || memoryValue < 256) {
      setError('请填写有效的 CPU 核数和至少 256 MB 的内存。')
      return
    }
    setError('')
    void createInstance({
      nodeID: node.id,
      name: name.trim(),
      imageId: imageID,
      imageVersion,
      cpu: cpuValue,
      memoryMB: memoryValue
    })
      .unwrap()
      .then(onClose)
      .catch(() => undefined)
  }

  return (
    <Dialog
      title="创建实例"
      onClose={onClose}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <label className={dialogLabelClass}>
          实例名称
          <input
            data-autofocus
            className={dialogFieldClass}
            value={name}
            maxLength={64}
            onChange={event => {
              setName(event.target.value)
              setError('')
            }}
            placeholder="例如：我的 AlemonX"
          />
        </label>
        <label className={dialogLabelClass}>
          审核镜像
          <select
            className={dialogFieldClass}
            value={imageID}
            disabled={catalogLoading || !catalog?.images.length}
            onChange={event => chooseImage(event.target.value)}
          >
            <option value="">{catalogLoading ? '正在加载镜像…' : '请选择镜像'}</option>
            {(catalog?.images ?? []).map(image => (
              <option key={image.id} value={image.id}>
                {image.name}
              </option>
            ))}
          </select>
        </label>
        <label className={dialogLabelClass}>
          版本
          <select
            className={dialogFieldClass}
            value={imageVersion}
            disabled={!selectedImage}
            onChange={event => {
              setImageVersion(event.target.value)
              setError('')
            }}
          >
            <option value="">请选择版本</option>
            {selectedVersions.map(version => (
              <option key={version.tag} value={version.tag}>
                {version.tag}
              </option>
            ))}
          </select>
        </label>
        <div className="rounded-lg border border-blue-100 bg-blue-50 px-3 py-2.5 text-xs leading-5 text-blue-800 dark:border-blue-900 dark:bg-blue-950 dark:text-blue-100">
          仅可选择平台审核并已发布的镜像版本。实例资源由节点 CPU、内存和数据盘共同决定。
        </div>
        <label className={dialogLabelClass}>
          CPU 核数
          <input
            className={dialogFieldClass}
            type="number"
            min="0.1"
            step="0.1"
            inputMode="decimal"
            value={cpu}
            onChange={event => {
              setCPU(event.target.value)
              setError('')
            }}
          />
        </label>
        <label className={dialogLabelClass}>
          内存（MB）
          <input
            className={dialogFieldClass}
            type="number"
            min="256"
            step="256"
            inputMode="numeric"
            value={memoryMB}
            onChange={event => {
              setMemoryMB(event.target.value)
              setError('')
            }}
          />
        </label>
      </div>
      <p className="mt-4 text-[11px] leading-5 text-slate-500 dark:text-slate-300">
        实例已分配 / 可分配上限：{node.cpuUsed} / {node.cpuQuota} 核，{node.memoryUsedMB} / {node.memoryQuotaMB} MB。
      </p>
      {error && <Alert tone="error">{error}</Alert>}
      <DialogFooter>
        <Button tone="secondary" disabled={submitting} onClick={onClose}>
          取消
        </Button>
        <Button
          loading={submitting}
          disabled={node.status !== 'online' || !node.ready || catalogLoading || !catalog?.images.length}
          onClick={submit}
        >
          {node.status !== 'online' ? '节点离线，暂不可创建' : !node.ready ? '节点需修复，暂不可创建' : '创建实例'}
        </Button>
      </DialogFooter>
    </Dialog>
  )
}
