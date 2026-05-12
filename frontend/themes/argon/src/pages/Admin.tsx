import { useState, useEffect, useRef } from 'react'
import {
  ArrowLeft, Eye, EyeOff, KeyRound, Loader2, ShieldCheck, AlertTriangle,
} from 'lucide-react'
import { api, getToken, setToken } from '../api'

// Argon does not implement the inner admin panel (Provider / Settings /
// Tasks / Config tabs).  Once authentication succeeds we transparently
// switch admin_theme to "default" so the server renders the full admin
// UI from the default theme on the next reload.  This is a one-shot
// hand-off: the admin can flip admin_theme back to argon via the default
// theme's settings page if they want to see the Argon login again.
//
// Safeguards against an accidental redirect loop:
//   1. If admin_theme is already "default", skip the PUT and just reload —
//      the server will serve the default theme next time, and Argon's
//      Admin won't run again.  No state mutation needed.
//   2. A sessionStorage attempt counter caps the number of PUT calls per
//      browser session.  If the third attempt is needed (i.e. the previous
//      two PUTs didn't take effect), we surface an explicit error instead
//      of spinning forever.
const HANDOFF_ATTEMPT_KEY = 'argon_admin_handoff_attempts'

async function handoffToDefaultAdmin() {
  const status = await api.themes()
  if (status.admin_theme === 'default') {
    // admin_theme is already default; Argon is being served for some
    // residual reason (cache, stale React Router nav).  Just reload and
    // let the server hand control to the default theme.
    sessionStorage.removeItem(HANDOFF_ATTEMPT_KEY)
    window.location.reload()
    return
  }
  const attempts = parseInt(sessionStorage.getItem(HANDOFF_ATTEMPT_KEY) ?? '0', 10)
  if (attempts >= 2) {
    throw new Error('管理面板主题切换未生效，请通过 default 主题的「设置 → 管理面板主题」手动切换')
  }
  sessionStorage.setItem(HANDOFF_ATTEMPT_KEY, String(attempts + 1))
  await api.updateAdminTheme('default')
  window.location.reload()
}

// ── Password input (module-scoped so identity is stable across renders) ──

function PasswordInput({ val, setVal, show, setShow, placeholder, autoFocus, onClearErr, onSubmit }: {
  val: string; setVal: (v: string) => void
  show: boolean; setShow: (v: boolean) => void
  placeholder: string; autoFocus?: boolean
  onClearErr: () => void
  onSubmit: () => void
}) {
  return (
    <div className="relative">
      <input
        type={show ? 'text' : 'password'}
        value={val}
        onChange={e => { setVal(e.target.value); onClearErr() }}
        onKeyDown={e => { if (e.key === 'Enter') onSubmit() }}
        placeholder={placeholder}
        className="argon-input pr-10"
        autoFocus={autoFocus}
      />
      <button
        type="button"
        onClick={() => setShow(!show)}
        className="absolute right-3 top-1/2 -translate-y-1/2 transition-colors"
        style={{ color: 'var(--argon-muted)' }}
        onMouseEnter={e => (e.currentTarget.style.color = 'var(--argon-primary)')}
        onMouseLeave={e => (e.currentTarget.style.color = 'var(--argon-muted)')}
        aria-label={show ? '隐藏' : '显示'}
      >
        {show ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
      </button>
    </div>
  )
}

// ── Argon login frame ───────────────────────────────────────────────────

function LoginFrame({ title, subtitle, children }: {
  title: string
  subtitle: string
  children: React.ReactNode
}) {
  return (
    <div className="min-h-screen">
      <header className="argon-hero pt-12 pb-32 px-6">
        <div className="max-w-7xl mx-auto relative z-10">
          <a
            href="/"
            className="inline-flex items-center gap-1.5 text-white/80 hover:text-white text-sm transition-colors mb-8 no-underline"
          >
            <ArrowLeft className="w-4 h-4" />
            返回仪表盘
          </a>
          <h1 className="text-3xl font-bold text-white mb-2">{title}</h1>
          <p className="text-white/70 max-w-xl">{subtitle}</p>
        </div>
      </header>

      <main className="max-w-md mx-auto px-6 -mt-24 relative z-10 pb-12">
        <div className="argon-card anim-argon-in">
          <div className="argon-card-body p-8 sm:p-10">
            {children}
          </div>
        </div>
        <p className="text-center text-xs mt-6" style={{ color: 'var(--argon-muted)' }}>
          Argon Theme · 管理员认证
        </p>
      </main>
    </div>
  )
}

// ── Change Token Form ───────────────────────────────────────────────────

function ChangeTokenForm({ onDone }: { onDone: () => void }) {
  const [value, setValue] = useState('')
  const [confirm, setConfirm] = useState('')
  const [showA, setShowA] = useState(false)
  const [showB, setShowB] = useState(false)
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)
  const clearErr = () => setErr('')

  const submit = async () => {
    const t = value.trim()
    if (t.length < 6) { setErr('密钥至少 6 位'); return }
    if (t !== confirm.trim()) { setErr('两次输入不一致'); return }
    setLoading(true); setErr('')
    try {
      await api.changeToken(t)
      setToken(t)
      onDone()
    } catch (e) {
      setErr(`修改失败：${(e as Error).message}`)
    } finally {
      setLoading(false)
    }
  }

  return (
    <LoginFrame title="首次登录" subtitle="检测到当前密钥为系统自动生成。出于安全考虑，请立即设置一个属于你的新密钥。">
      <div className="flex items-center gap-2 mb-6">
        <div className="w-10 h-10 rounded-lg flex items-center justify-center" style={{ background: 'rgba(251,99,64,.12)', color: 'var(--argon-warning)' }}>
          <KeyRound className="w-5 h-5" />
        </div>
        <h2 className="text-base font-semibold" style={{ color: 'var(--argon-heading)' }}>修改管理密钥</h2>
      </div>

      <div className="space-y-3 mb-4">
        <PasswordInput
          val={value} setVal={setValue} show={showA} setShow={setShowA}
          placeholder="新密钥（至少 6 位）" autoFocus
          onClearErr={clearErr} onSubmit={submit}
        />
        <PasswordInput
          val={confirm} setVal={setConfirm} show={showB} setShow={setShowB}
          placeholder="再次输入新密钥"
          onClearErr={clearErr} onSubmit={submit}
        />
      </div>

      {err && (
        <p className="text-xs mb-3 px-3 py-2 rounded" style={{ color: 'var(--argon-danger)', background: 'rgba(245,54,92,.08)' }}>
          {err}
        </p>
      )}

      <button onClick={submit} disabled={loading} className="argon-btn argon-btn-primary w-full justify-center">
        {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <ShieldCheck className="w-4 h-4" />}
        {loading ? '保存中…' : '确认修改并进入'}
      </button>
    </LoginFrame>
  )
}

// ── Token Gate (login) ──────────────────────────────────────────────────

function TokenGate({ onEnter }: { onEnter: () => void }) {
  const [value, setValue] = useState('')
  const [show, setShow] = useState(false)
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)
  const [tokenRequired, setTokenRequired] = useState(false)
  const [firstUse, setFirstUse] = useState(false)
  const clearErr = () => setErr('')

  const submit = async () => {
    const token = value.trim()
    if (tokenRequired && !token) { setErr('服务器已设置 ADMIN_TOKEN，不能留空'); return }
    setLoading(true); setErr('')
    setToken(token)
    try {
      const state = await api.detection()
      if (state.first_use) setFirstUse(true)
      else onEnter()
    } catch (e) {
      setToken('')
      const msg = (e as Error).message ?? ''
      if (msg.toLowerCase().includes('unauthorized') || msg === '401') {
        setTokenRequired(true)
        setErr('Token 错误或未提供，请检查后重试')
      } else {
        setErr(`验证失败：${msg}`)
      }
    } finally {
      setLoading(false)
    }
  }

  if (firstUse) return <ChangeTokenForm onDone={onEnter} />

  return (
    <LoginFrame title="管理员认证" subtitle="输入 ADMIN_TOKEN 访问管理面板。若服务运行在 localhost 且未设置 Token，可直接留空进入。">
      <div className="flex items-center gap-2 mb-6">
        <div className="w-10 h-10 rounded-lg flex items-center justify-center" style={{ background: 'rgba(94,114,228,.12)', color: 'var(--argon-primary)' }}>
          <ShieldCheck className="w-5 h-5" />
        </div>
        <h2 className="text-base font-semibold" style={{ color: 'var(--argon-heading)' }}>访问令牌</h2>
      </div>

      <div className="mb-4">
        <PasswordInput
          val={value} setVal={setValue} show={show} setShow={setShow}
          placeholder={tokenRequired ? 'Admin Token（必填）' : 'Admin Token（可为空）'}
          autoFocus
          onClearErr={clearErr} onSubmit={submit}
        />
      </div>

      {err && (
        <p className="text-xs mb-3 px-3 py-2 rounded" style={{ color: 'var(--argon-danger)', background: 'rgba(245,54,92,.08)' }}>
          {err}
        </p>
      )}

      <button onClick={submit} disabled={loading} className="argon-btn argon-btn-primary w-full justify-center">
        {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <ShieldCheck className="w-4 h-4" />}
        {loading ? '验证中…' : '进入管理面板'}
      </button>
    </LoginFrame>
  )
}

// ── Hand-off screen: shown briefly while we swap admin_theme to default ──

function HandingOff({ onRetry, onLogout }: { onRetry: () => void; onLogout: () => void }) {
  const [err, setErr] = useState('')
  const [retrying, setRetrying] = useState(false)
  const triggeredRef = useRef(false)

  useEffect(() => {
    if (triggeredRef.current) return
    triggeredRef.current = true
    handoffToDefaultAdmin().catch(e => setErr((e as Error).message))
  }, [])

  const retry = async () => {
    setRetrying(true); setErr('')
    try { await handoffToDefaultAdmin() }
    catch (e) {
      setErr((e as Error).message)
      setRetrying(false)
      onRetry()
    }
  }

  if (!err) {
    return (
      <div className="min-h-screen flex flex-col items-center justify-center gap-3" style={{ color: 'var(--argon-muted)' }}>
        <Loader2 className="w-7 h-7 animate-spin" style={{ color: 'var(--argon-primary)' }} />
        <p className="text-sm">正在打开管理面板…</p>
      </div>
    )
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <div className="argon-card max-w-md w-full anim-argon-in">
        <div className="argon-card-body p-8 text-center">
          <div className="w-12 h-12 rounded-lg flex items-center justify-center mx-auto mb-4" style={{ background: 'rgba(245,54,92,.12)', color: 'var(--argon-danger)' }}>
            <AlertTriangle className="w-6 h-6" />
          </div>
          <h2 className="text-base font-semibold mb-2" style={{ color: 'var(--argon-heading)' }}>无法打开管理面板</h2>
          <p className="text-sm mb-4 break-all" style={{ color: 'var(--argon-text)' }}>{err}</p>
          <div className="flex gap-2 justify-center">
            <button onClick={retry} disabled={retrying} className="argon-btn argon-btn-primary">
              {retrying ? <Loader2 className="w-4 h-4 animate-spin" /> : null}
              {retrying ? '重试中…' : '重试'}
            </button>
            <button onClick={onLogout} className="argon-btn argon-btn-secondary">退出登录</button>
          </div>
          <a href="/" className="block text-xs mt-4 no-underline" style={{ color: 'var(--argon-muted)' }}>
            <ArrowLeft className="inline w-3 h-3 mr-1" />返回仪表盘
          </a>
        </div>
      </div>
    </div>
  )
}

// ── Outer admin shell ───────────────────────────────────────────────────

export default function Admin() {
  const [authed, setAuthed] = useState(false)
  const [verifying, setVerifying] = useState(!!getToken())

  useEffect(() => {
    if (!getToken()) return
    api.detection()
      .then(s => { if (!s.first_use) setAuthed(true) })
      .catch(() => setToken(''))
      .finally(() => setVerifying(false))
  }, [])

  if (verifying) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Loader2 className="w-6 h-6 animate-spin" style={{ color: 'var(--argon-primary)' }} />
      </div>
    )
  }

  if (!authed) return <TokenGate onEnter={() => setAuthed(true)} />

  const logout = () => { setToken(''); setAuthed(false) }
  return <HandingOff onRetry={() => { /* HandingOff re-renders error UI */ }} onLogout={logout} />
}
