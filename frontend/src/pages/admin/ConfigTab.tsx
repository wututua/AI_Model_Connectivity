import { useState } from 'react'
import { Download, FileJson, RefreshCw, Upload } from 'lucide-react'
import { api } from '../../api'
import type { ConfigExport, ConfigImport } from '../../types'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '../../components/ui/alert-dialog'
import { Button } from '../../components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../../components/ui/card'
import { Textarea } from '../../components/ui/textarea'
import { Feedback, LoadingButton, useAutoMsg } from './shared'

export function ConfigTab() {
  const [exportData, setExportData] = useState('')
  const [importText, setImportText] = useState('')
  const [loading, setLoading] = useState<string | null>(null)
  const [confirmImport, setConfirmImport] = useState(false)
  const [message, setMessage] = useAutoMsg()

  const exportConfig = async () => {
    setLoading('export'); setMessage('')
    try {
      const data: ConfigExport = await api.exportConfig()
      const json = JSON.stringify(data, null, 2)
      setExportData(json)
      const url = URL.createObjectURL(new Blob([json], { type: 'application/json' }))
      const link = document.createElement('a')
      link.href = url; link.download = `model-connectivity-config-${new Date().toISOString().slice(0, 10)}.json`; link.click()
      URL.revokeObjectURL(url)
      setMessage('配置已导出')
    } catch (cause) { setMessage(`错误：${(cause as Error).message}`) }
    finally { setLoading(null) }
  }

  const importConfig = async () => {
    setLoading('import'); setMessage('')
    try {
      const parsed = JSON.parse(importText) as ConfigImport
      if (!parsed || typeof parsed !== 'object' || !parsed.settings || !Array.isArray(parsed.providers)) throw new Error('配置必须包含 settings 和 providers')
      await api.importConfig(parsed)
      setConfirmImport(false); setMessage('配置已导入并生效')
    } catch (cause) { setConfirmImport(false); setMessage(`错误：${(cause as Error).message}`) }
    finally { setLoading(null) }
  }

  const reload = async () => {
    setLoading('reload'); setMessage('')
    try { await api.reloadConfig(); setMessage('已从 .env 重新加载配置') }
    catch (cause) { setMessage(`错误：${(cause as Error).message}`) }
    finally { setLoading(null) }
  }

  return (
    <div className="space-y-5">
      <Feedback message={message} />
      <div className="grid gap-4 xl:grid-cols-2">
        <Card>
          <CardHeader><CardTitle className="flex items-center gap-2"><Download className="size-4" />导出配置</CardTitle><CardDescription>下载当前设置和 Provider 列表。导出内容不包含 API Key。</CardDescription></CardHeader>
          <CardContent className="space-y-4"><LoadingButton onClick={exportConfig} loading={loading === 'export'}><Download />导出 JSON</LoadingButton>{exportData && <Textarea readOnly value={exportData} className="min-h-72 resize-none font-mono text-xs" aria-label="导出的配置内容" />}</CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle className="flex items-center gap-2"><Upload className="size-4" />导入配置</CardTitle><CardDescription>粘贴配置 JSON。未提供的新 API Key 会保留服务端现有值。</CardDescription></CardHeader>
          <CardContent className="space-y-4"><Textarea value={importText} onChange={event => setImportText(event.target.value)} placeholder={'{\n  "settings": { ... },\n  "providers": [ ... ]\n}'} className="min-h-72 resize-none font-mono text-xs" /><Button onClick={() => setConfirmImport(true)} disabled={!importText.trim() || loading !== null}><Upload />导入配置</Button></CardContent>
        </Card>
      </div>
      <Card>
        <CardHeader className="flex-row items-start justify-between space-y-0"><div><CardTitle className="flex items-center gap-2"><FileJson className="size-4" />重新加载 .env</CardTitle><CardDescription className="mt-2">重新读取环境配置并热加载，不需要重启服务。</CardDescription></div><LoadingButton variant="outline" onClick={reload} loading={loading === 'reload'}><RefreshCw />重新加载</LoadingButton></CardHeader>
      </Card>

      <AlertDialog open={confirmImport} onOpenChange={setConfirmImport}>
        <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>导入并覆盖当前配置？</AlertDialogTitle><AlertDialogDescription>导入会更新运行设置和 Provider 列表，并立即影响后续检测任务。建议先导出当前配置作为备份。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel disabled={loading === 'import'}>取消</AlertDialogCancel><AlertDialogAction onClick={event => { event.preventDefault(); importConfig() }} disabled={loading === 'import'}>{loading === 'import' ? '导入中…' : '确认导入'}</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
