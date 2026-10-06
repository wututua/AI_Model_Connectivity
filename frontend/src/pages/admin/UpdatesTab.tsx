import { useCallback, useEffect, useRef, useState } from 'react'
import { Download, ExternalLink, RefreshCw, RotateCw } from 'lucide-react'
import { api, APIError } from '../../api'
import type { SystemUpdateCheck, SystemUpdateStatus } from '../../types'
import { Button } from '../../components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '../../components/ui/alert-dialog'
import { Feedback, Field, LoadingButton } from './shared'

const stages: Record<string, string> = {
  queued: '等待更新服务', checking: '核对版本', downloading: '下载发布包', verifying: '校验发布包',
  backup: '停服备份', installing: '替换程序与前端', restarting: '启动及健康检查', restoring: '恢复旧版本',
  restored: '已恢复旧版本', complete: '更新完成',
}
const results: Record<string, string> = { pending: '等待执行', running: '更新中', succeeded: '更新成功', failed: '更新失败', rolled_back: '已回滚', recovery_required: '需要手动恢复' }
const active = (value: SystemUpdateStatus | null) => !!value?.job && ['pending', 'running', 'recovery_required'].includes(value.job.status)
type Submission = { id: string; version: string; channel: 'stable' | 'preview'; abort: AbortController; accepted: boolean }

export function UpdatesTab() {
  const [status, setStatus] = useState<SystemUpdateStatus | null>(null)
  const [channel, setChannel] = useState<'stable' | 'preview'>('stable')
  const [check, setCheck] = useState<SystemUpdateCheck | null>(null)
  const [loading, setLoading] = useState(true)
  const [checking, setChecking] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [confirm, setConfirm] = useState(false)
  const [message, setMessage] = useState('')
  const [connectionError, setConnectionError] = useState('')
  const [uncertain, setUncertain] = useState(false)
  const mounted = useRef(false)
  const statusSequence = useRef(0)
  const checkSequence = useRef(0)
  const checkAbort = useRef<AbortController>()
  const statusAbort = useRef<AbortController>()
  const locked = useRef(false)
  const submission = useRef<Submission>()
  const unresolved = useRef<Submission>()
  const observedVersion = useRef<string>()
  const statusPending = useRef(false)

  const load = useCallback(async (force = false) => {
    if (statusPending.current && !force) return
    statusPending.current = true
    const sequence = ++statusSequence.current
    statusAbort.current?.abort()
    const abort = new AbortController()
    statusAbort.current = abort
    let timedOut = false
    const timeout = setTimeout(() => { timedOut = true; abort.abort() }, 15000)
    const resolving = unresolved.current
    try {
      const value = resolving
        ? await api.resolveUpdate(resolving.id, abort.signal)
        : await api.updateStatus(abort.signal)
      if (!mounted.current || sequence !== statusSequence.current) return
      if (observedVersion.current && observedVersion.current !== value.version) {
        checkSequence.current++; checkAbort.current?.abort()
        setCheck(null); setChecking(false); setConfirm(false)
      }
      observedVersion.current = value.version
      setStatus(value); setConnectionError('')
      if (resolving && unresolved.current === resolving) {
        // The server invalidated any delayed, unqueued POST before returning.
        unresolved.current = undefined
        setUncertain(false)
        const matching = value.job?.id === resolving.id && value.job.version === resolving.version && value.job.channel === resolving.channel
        setMessage(matching ? '' : value.job?.id === resolving.id ? '另一个更新请求已受理。' : '提交未排队或已结束，旧请求已失效。')
      }
      const attempt = submission.current
      if (attempt && value.job?.id === attempt.id && value.job.version === attempt.version && value.job.channel === attempt.channel) {
        attempt.accepted = true
        setConfirm(false)
        attempt.abort.abort()
      }
    } catch (e) {
      if (mounted.current && sequence === statusSequence.current && (!abort.signal.aborted || timedOut)) {
        setConnectionError(timedOut ? '状态查询超时，正在重试。' : `连接暂不可用，正在重试：${(e as Error).message}`)
      }
    } finally {
      clearTimeout(timeout)
      if (sequence === statusSequence.current) {
        statusPending.current = false
        if (mounted.current) setLoading(false)
      }
    }
  }, [])

  useEffect(() => {
    mounted.current = true
    void load()
    const timer = setInterval(() => void load(), 3000)
    return () => {
      mounted.current = false; statusSequence.current++; checkSequence.current++
      statusPending.current = false
      statusAbort.current?.abort(); checkAbort.current?.abort(); submission.current?.abort.abort(); clearInterval(timer)
    }
  }, [load])

  const findUpdate = useCallback(async () => {
    if (locked.current) return
    const sequence = ++checkSequence.current
    checkAbort.current?.abort()
    const abort = new AbortController()
    checkAbort.current = abort
    setChecking(true); setMessage(''); setCheck(null)
    try {
      const value = await api.checkUpdate(channel, abort.signal)
      if (mounted.current && checkSequence.current === sequence) setCheck(value)
    } catch (e) {
      if (mounted.current && checkSequence.current === sequence && !abort.signal.aborted) setMessage(`错误：检查失败，${(e as Error).message}`)
    } finally { if (mounted.current && checkSequence.current === sequence) setChecking(false) }
  }, [channel])

  useEffect(() => { void findUpdate() }, [findUpdate])

  const selectChannel = (value: 'stable' | 'preview') => {
    checkSequence.current++; checkAbort.current?.abort()
    setChannel(value); setCheck(null); setChecking(false); setMessage(''); setConfirm(false)
  }
  const canUpdate = !!status?.supported && !!status.request_id && !active(status) && !uncertain && !connectionError &&
    !checking && check?.channel === channel && !!check.available && !!check.release?.package_available && status.version !== check.release.version

  const install = async () => {
    if (locked.current || !canUpdate || !check?.release || !status) return
    locked.current = true
    const attempt: Submission = { id: status.request_id, version: check.release.version, channel, abort: new AbortController(), accepted: false }
    submission.current = attempt
    let timedOut = false
    const timeout = setTimeout(() => { timedOut = true; attempt.abort.abort() }, 25000)
    statusSequence.current++; statusAbort.current?.abort(); statusPending.current = false
    setSubmitting(true); setMessage('')
    try {
      const job = await api.startUpdate(attempt.channel, attempt.version, attempt.id, attempt.abort.signal)
      if (job.id !== attempt.id || job.version !== attempt.version || job.channel !== attempt.channel) throw new Error('更新任务凭据或目标不匹配')
      if (mounted.current && !attempt.accepted) {
        attempt.accepted = true
        setStatus(value => value ? { ...value, job } : value)
        setConfirm(false); setUncertain(false)
      }
    } catch (e) {
      if (mounted.current && !attempt.accepted) {
        setConfirm(false)
        const ambiguous = !(e instanceof APIError) || e.status >= 500
        unresolved.current = ambiguous ? attempt : undefined
        setUncertain(ambiguous)
        setMessage(ambiguous
          ? `错误：${timedOut ? '提交响应超时' : `未能确认提交结果，${(e as Error).message}`}。正在确认任务状态。`
          : `错误：更新请求被拒绝，${(e as Error).message}`)
      }
    } finally {
      clearTimeout(timeout)
      if (submission.current === attempt) submission.current = undefined
      locked.current = false
      if (mounted.current) { setSubmitting(false); void load(true) }
    }
  }

  const release = check?.release
  const stale = status?.job && active(status) && Date.now() - Date.parse(status.job.updated_at) > 30 * 60 * 1000
  return <div className="space-y-6">
    <Feedback message={message} />
    {connectionError && <p role="alert" className="text-sm text-warning">{connectionError}</p>}
    <dl className="grid grid-cols-1 gap-4 border-b pb-5 sm:grid-cols-3">
      <div><dt className="text-xs text-muted-foreground">当前版本</dt><dd className="mt-1 break-all font-mono text-sm">{status?.version ?? (loading ? '正在加载' : '未知')}</dd></div>
      <div><dt className="text-xs text-muted-foreground">运行平台</dt><dd className="mt-1 break-all text-sm">{status?.platform ?? '-'}</dd></div>
      <div><dt className="text-xs text-muted-foreground">部署方式</dt><dd className="mt-1 text-sm">{status ? ({ systemd: 'Linux / systemd', docker: 'Docker', manual: '手动部署' }[status.deployment] ?? status.deployment) : '-'}</dd></div>
    </dl>
    <div className="flex flex-wrap items-end gap-3">
      <div className="w-52 max-w-full"><Field label="更新通道"><Select value={channel} onValueChange={value => selectChannel(value as 'stable' | 'preview')} disabled={submitting || active(status)}>
        <SelectTrigger aria-label="更新通道"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="stable">稳定版</SelectItem><SelectItem value="preview">预发布（含 beta / RC）</SelectItem></SelectContent>
      </Select></Field></div>
      <LoadingButton loading={checking} disabled={submitting} onClick={() => void findUpdate()}><RefreshCw />检查更新</LoadingButton>
      <Button variant="outline" size="icon" aria-label="刷新更新状态" title="刷新更新状态" onClick={() => void load(true)}><RotateCw /></Button>
    </div>
    {check && <section className="space-y-3 border-y py-5" aria-label="可用版本">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0"><h2 className="break-all text-base font-semibold">{release ? release.version : '该通道暂无发布版本'}</h2>
          {release && <p className="mt-1 text-sm text-muted-foreground">{release.prerelease ? '预发布版本' : '正式版本'} · {check.available ? '有新版本' : status?.version === 'dev' ? '开发构建，不能自动升级' : '没有可升级的新版本'}</p>}
        </div>
        <div className="flex flex-wrap gap-2">
          {release && <Button asChild variant="outline"><a href={release.url} target="_blank" rel="noreferrer"><ExternalLink />发布详情</a></Button>}
          {status?.supported && <Button disabled={!canUpdate || submitting} onClick={() => setConfirm(true)}><Download />安装更新</Button>}
        </div>
      </div>
      {release && !release.package_available && <p className="text-sm text-warning">该版本缺少当前平台发布包或校验文件，暂不能安装。</p>}
      {release?.notes && <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted/40 p-4 font-sans text-sm leading-relaxed [overflow-wrap:anywhere]">{release.notes}</pre>}
      <p className="text-xs text-muted-foreground">检查时间：{new Date(check.checked_at).toLocaleString('zh-CN')}</p>
    </section>}
    {status && !status.supported && <section className="space-y-3 border-t pt-4">
      <p className="text-sm text-muted-foreground">{status.reason}</p>
      {status.deployment === 'docker'
        ? <p className="text-sm">使用目标版本镜像重建容器前，请停服备份数据库。源码构建的 Compose 部署需先更新源码并重新构建。</p>
        : status.platform.startsWith('linux/') && status.version !== 'dev'
          ? <code className="block break-words rounded-md bg-muted/40 p-3 text-xs [overflow-wrap:anywhere]">sudo bash install-model-connectivity.sh enable-updates</code>
          : null}
    </section>}
    {status?.job && <section className="space-y-3 border-t pt-4" aria-label="更新任务">
      <div className="flex flex-wrap items-center justify-between gap-2"><h2 className="text-sm font-semibold">{results[status.job.status] ?? '状态未知'}</h2><span className="break-all font-mono text-sm">{status.job.version}</span></div>
      <p role="status" className="text-sm">{stages[status.job.stage] ?? status.job.stage}</p>
      <p className="break-words text-sm text-muted-foreground">{status.job.message}</p>
      <p className="break-all text-xs text-muted-foreground">#{status.job.id} · {new Date(status.job.updated_at).toLocaleString('zh-CN')}</p>
      {(stale || status.job.status === 'recovery_required') && <p role="alert" className="text-sm text-destructive">任务结果尚未确认。请在服务器检查更新服务日志及备份，不要直接重试。</p>}
      {['failed', 'recovery_required'].includes(status.job.status) && <code className="block break-all rounded-md bg-muted/40 p-3 text-xs">sudo journalctl -u model-connectivity-update.service -n 100 --no-pager</code>}
      {status.job.status === 'succeeded' && <Button variant="outline" onClick={() => window.location.reload()}><RefreshCw />重新加载页面</Button>}
    </section>}
    <AlertDialog open={confirm} onOpenChange={open => { if (!submitting) setConfirm(open) }}>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>安装 {release?.version}？</AlertDialogTitle>
        <AlertDialogDescription>服务会短暂停止，更新前备份程序与数据库。启动失败时尝试恢复整个备份。请确保磁盘空间充足，且没有其他进程写入同一数据库。{release?.prerelease ? ' 当前选择为预发布版本。' : ''}</AlertDialogDescription>
      </AlertDialogHeader><AlertDialogFooter><AlertDialogCancel disabled={submitting}>取消</AlertDialogCancel><AlertDialogAction disabled={!canUpdate || submitting} onClick={e => { e.preventDefault(); void install() }}>{submitting ? '正在提交' : '确认更新'}</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
    </AlertDialog>
  </div>
}
