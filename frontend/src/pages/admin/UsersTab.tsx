import { useCallback, useEffect, useState } from 'react'
import { Pencil, Plus, RefreshCw, Search, ShieldCheck, Trash2, UserRound } from 'lucide-react'
import { api } from '../../api'
import { useAuth } from '../../hooks/useAuth'
import type { User, UserInput } from '../../types'
import { passwordError } from '../../utils/password'
import { PasswordInput } from '../../components/PasswordInput'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import { Input } from '../../components/ui/input'
import { Switch } from '../../components/ui/switch'
import { Label } from '../../components/ui/label'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '../../components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select'
import { AlertDialog, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogCancel } from '../../components/ui/alert-dialog'
import { Feedback, Field, ListSkeleton, LoadingButton, useAutoMsg } from './shared'

export function UsersTab() {
  const { session } = useAuth()
  const [users, setUsers] = useState<User[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')
  const [editing, setEditing] = useState<User | 'new' | null>(null)
  const [deleting, setDeleting] = useState<User | null>(null)
  const [busy, setBusy] = useState(false)
  const [deleteError, setDeleteError] = useState('')
  const [message, setMessage] = useAutoMsg()
  const load = useCallback(async () => {
    setLoading(true)
    try { setUsers(await api.users()) }
    catch (cause) { setMessage(`加载失败：${(cause as Error).message}`) }
    finally { setLoading(false) }
  }, [setMessage])
  useEffect(() => { void load() }, [load])
  const remove = async () => {
    if (!deleting || busy) return
    setBusy(true); setDeleteError('')
    try { await api.deleteUser(deleting.id); setDeleting(null); setMessage('用户已删除'); await load() }
    catch (cause) { setDeleteError((cause as Error).message) }
    finally { setBusy(false) }
  }
  const filtered = users.filter(user => user.username.includes(search.trim().toLowerCase()))
  return <div className="space-y-4">
    <div className="flex flex-wrap justify-between gap-3">
      <div className="relative w-full sm:max-w-xs"><Search className="absolute left-3 top-2.5 size-4 text-muted-foreground" /><Input aria-label="搜索用户" placeholder="搜索账号" value={search} onChange={event => setSearch(event.target.value)} className="pl-9" /></div>
      <div className="flex gap-2"><Button variant="outline" size="icon" onClick={load} disabled={loading} aria-label="刷新用户" title="刷新用户"><RefreshCw className={loading ? 'animate-spin' : ''} /></Button><Button onClick={() => setEditing('new')}><Plus />新增用户</Button></div>
    </div>
    <Feedback message={message} />
    {loading && !users.length ? <ListSkeleton label="正在加载用户" /> : <div className="divide-y border-y">
      {filtered.map(user => <div key={user.id} className="flex flex-wrap items-center gap-3 py-4">
        <span className="hidden size-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground sm:flex">{user.role === 'admin' ? <ShieldCheck className="size-4" /> : <UserRound className="size-4" />}</span>
        <div className="min-w-0 flex-1"><p className="break-all text-sm font-semibold">{user.username}{user.id === session.user?.id && <span className="ml-2 text-xs font-normal text-muted-foreground">当前账号</span>}</p><p className="mt-1 text-xs text-muted-foreground">{user.role === 'admin' ? '管理员' : '普通用户'}{user.must_change_password && ' · 待修改初始密码'}</p></div>
        <Badge variant={user.enabled ? 'success' : 'muted'} className="shrink-0 whitespace-nowrap">{user.enabled ? '已启用' : '已禁用'}</Badge>
        <div className="flex shrink-0 gap-1">
          <Button variant="ghost" size="icon" disabled={user.id === session.user?.id} onClick={() => setEditing(user)} aria-label={`编辑 ${user.username}`} title="编辑用户"><Pencil /></Button>
          <Button variant="ghost" size="icon" disabled={user.id === session.user?.id} className="text-destructive" onClick={() => { setDeleting(user); setDeleteError('') }} aria-label={`删除 ${user.username}`} title="删除用户"><Trash2 /></Button>
        </div>
      </div>)}
      {!filtered.length && <p className="py-12 text-center text-sm text-muted-foreground">没有匹配的用户</p>}
    </div>}
    <UserEditor key={editing === 'new' ? 'new' : editing?.id ?? 'closed'} user={editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); setMessage('用户已保存'); void load() }} />
    <AlertDialog open={Boolean(deleting)} onOpenChange={open => { if (!open && !busy) setDeleting(null) }}>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle className="break-all">删除用户 {deleting?.username}？</AlertDialogTitle><AlertDialogDescription>该用户的所有登录会话将立即失效，此操作无法撤销。</AlertDialogDescription></AlertDialogHeader>
        {deleteError && <p role="alert" className="text-sm text-destructive">{deleteError}</p>}
        <AlertDialogFooter><AlertDialogCancel disabled={busy}>取消</AlertDialogCancel><LoadingButton variant="destructive" loading={busy} onClick={remove}><Trash2 />确认删除</LoadingButton></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
}

function UserEditor({ user, onClose, onSaved }: { user: User | 'new' | null; onClose: () => void; onSaved: () => void }) {
  const existing = user && user !== 'new' ? user : null
  const [form, setForm] = useState<UserInput>({ username: existing?.username ?? '', password: '', role: existing?.role ?? 'user', enabled: existing?.enabled ?? true })
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (busy) return
    if (!existing || form.password) {
      const invalid = passwordError(form.password)
      if (invalid) { setError(invalid); return }
    }
    setBusy(true); setError('')
    try { if (existing) await api.updateUser(existing.id, form); else await api.createUser(form); onSaved() }
    catch (cause) { setError((cause as Error).message) }
    finally { setBusy(false) }
  }
  return <Dialog open={Boolean(user)} onOpenChange={open => { if (!open && !busy) onClose() }}>
    <DialogContent className="max-h-[90dvh] overflow-y-auto">
      <DialogHeader><DialogTitle>{existing ? '编辑用户' : '新增用户'}</DialogTitle><DialogDescription>{existing ? '保存后该用户的现有会话将失效。密码留空则保持不变。' : '新用户首次登录时需要修改初始密码。'}</DialogDescription></DialogHeader>
      <form onSubmit={submit} className="space-y-4">
        <fieldset disabled={busy} className="space-y-4">
          <Field label="账号" htmlFor="user-username" hint="3-32 位字母、数字、点、下划线或短横线"><Input id="user-username" autoComplete="off" value={form.username} onChange={event => setForm({ ...form, username: event.target.value })} required maxLength={32} /></Field>
          <Field label="用户组" htmlFor="user-role"><Select value={form.role} onValueChange={role => setForm({ ...form, role: role as User['role'] })} disabled={busy}><SelectTrigger id="user-role"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="user">普通用户</SelectItem><SelectItem value="admin">管理员</SelectItem></SelectContent></Select></Field>
          <Field label={existing ? '重置密码' : '初始密码'} htmlFor="user-password" hint="至少 8 位，包含大写字母、小写字母和数字"><PasswordInput id="user-password" autoComplete="new-password" value={form.password} onChange={event => setForm({ ...form, password: event.target.value })} required={!existing} /></Field>
          <div className="flex items-center justify-between py-2"><Label htmlFor="user-enabled">启用账号</Label><Switch id="user-enabled" checked={form.enabled} onCheckedChange={enabled => setForm({ ...form, enabled })} disabled={busy} /></div>
        </fieldset>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <DialogFooter><Button type="button" variant="outline" disabled={busy} onClick={onClose}>取消</Button><LoadingButton type="submit" loading={busy}>{existing ? '保存用户' : '创建用户'}</LoadingButton></DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
}
