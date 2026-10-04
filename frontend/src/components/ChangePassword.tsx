import { useState } from 'react'
import { KeyRound } from 'lucide-react'
import { api } from '../api'
import { useAuth } from '../hooks/useAuth'
import { passwordError } from '../utils/password'
import { PasswordInput } from './PasswordInput'
import { Field, LoadingButton } from '../pages/admin/shared'

export function ChangePassword() {
  const { accept, session } = useAuth()
  const [current, setCurrent] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)
  const [busy, setBusy] = useState(false)
  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (busy) return
    const invalid = passwordError(password)
    if (invalid) { setError(invalid); return }
    if (password !== confirm) { setError('两次输入的密码不一致'); return }
    setBusy(true); setError(''); setSaved(false)
    try {
      accept(await api.changePassword(current, password))
      setCurrent(''); setPassword(''); setConfirm(''); setSaved(true)
    } catch (cause) { setError((cause as Error).message) }
    finally { setBusy(false) }
  }
  return <form onSubmit={submit} className="max-w-md space-y-4">
    <input type="text" autoComplete="username" value={session.user?.username ?? ''} readOnly className="sr-only" tabIndex={-1} aria-label="当前账号" />
    <fieldset disabled={busy} className="space-y-4">
      <Field label="当前密码" htmlFor="current-password"><PasswordInput id="current-password" value={current} onChange={event => setCurrent(event.target.value)} autoComplete="current-password" required /></Field>
      <Field label="新密码" htmlFor="new-password" hint="至少 8 位，包含大写字母、小写字母和数字，无需特殊符号"><PasswordInput id="new-password" value={password} onChange={event => setPassword(event.target.value)} autoComplete="new-password" required /></Field>
      <Field label="确认新密码" htmlFor="confirm-password"><PasswordInput id="confirm-password" value={confirm} onChange={event => setConfirm(event.target.value)} autoComplete="new-password" required /></Field>
    </fieldset>
    {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    {saved && <p role="status" className="text-sm text-success">密码已更新，其他登录会话已退出</p>}
    <LoadingButton type="submit" loading={busy}><KeyRound />更新密码</LoadingButton>
  </form>
}
