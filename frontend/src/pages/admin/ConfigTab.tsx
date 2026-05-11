import { useState } from 'react'
import { Download, Upload, RotateCcw } from 'lucide-react'
import { api } from '../../api'
import type { ConfigExport } from '../../types'
import { useAutoMsg, Btn, inputCls } from './shared'

export function ConfigTab() {
  const [exportData, setExportData] = useState('')
  const [importText, setImportText] = useState('')
  const [loading, setLoading] = useState<string | null>(null)
  const [msg, setMsg] = useAutoMsg()

  const withLoad = async (key: string, fn: () => Promise<unknown>, successMsg: string) => {
    setLoading(key)
    setMsg('')
    try {
      await fn()
      setMsg(successMsg)
    } catch (e) {
      setMsg(`错误：${(e as Error).message}`)
    } finally {
      setLoading(null)
    }
  }

  const doExport = async () => {
    setLoading('export')
    setMsg('')
    try {
      const data: ConfigExport = await api.exportConfig()
      const json = JSON.stringify(data, null, 2)
      setExportData(json)
      const blob = new Blob([json], { type: 'application/json' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url; a.download = 'cg-config.json'; a.click()
      URL.revokeObjectURL(url)
      setMsg('配置已导出')
    } catch (e) {
      setMsg(`错误：${(e as Error).message}`)
    } finally {
      setLoading(null)
    }
  }

  const doImport = () => withLoad('import', async () => {
    const parsed = JSON.parse(importText)
    await api.importConfig(parsed)
  }, '配置已导入')

  const doReload = () => withLoad('reload', () => api.reloadConfig(), '已从 .env 重新加载配置')

  return (
    <div className="space-y-6 max-w-2xl">
      <h2 className="text-base font-semibold" style={{ color: 'var(--text)' }}>配置管理</h2>

      {msg && <p className="text-sm font-mono" style={{ color: msg.startsWith('错误') ? 'var(--error)' : 'var(--ok)' }}>{msg}</p>}

      <section className="glass rounded-[22px] p-5 space-y-3 anim-scale-in" style={{ animationDelay: '40ms' }}>
        <h3 className="text-sm font-medium" style={{ color: 'var(--text)' }}>导出配置</h3>
        <p className="text-xs" style={{ color: 'var(--muted)' }}>下载当前设置和 Provider 列表（不含 API Key）为 JSON 文件</p>
        <Btn onClick={doExport} loading={loading === 'export'} variant="default">
          <Download className="w-3.5 h-3.5" />导出 JSON
        </Btn>
        {exportData && (
          <textarea
            readOnly
            value={exportData}
            className={`${inputCls} h-32 resize-none text-[11px]`}
          />
        )}
      </section>

      <section className="glass rounded-[22px] p-5 space-y-3 anim-scale-in" style={{ animationDelay: '110ms' }}>
        <h3 className="text-sm font-medium" style={{ color: 'var(--text)' }}>导入配置</h3>
        <p className="text-xs" style={{ color: 'var(--muted)' }}>粘贴 JSON 配置并导入；已有 API Key 将自动保留</p>
        <textarea
          value={importText}
          onChange={e => setImportText(e.target.value)}
          placeholder='{"settings":{...},"providers":[...]}'
          className={`${inputCls} h-40 resize-none text-[11px]`}
        />
        <Btn onClick={doImport} loading={loading === 'import'} variant="primary" disabled={!importText.trim()}>
          <Upload className="w-3.5 h-3.5" />导入
        </Btn>
      </section>

      <section className="glass rounded-[22px] p-5 space-y-3 anim-scale-in" style={{ animationDelay: '180ms' }}>
        <h3 className="text-sm font-medium" style={{ color: 'var(--text)' }}>热加载 .env</h3>
        <p className="text-xs" style={{ color: 'var(--muted)' }}>重新读取 .env 文件并热加载配置，不重启服务</p>
        <Btn onClick={doReload} loading={loading === 'reload'} variant="default">
          <RotateCcw className="w-3.5 h-3.5" />重载 .env
        </Btn>
      </section>
    </div>
  )
}
