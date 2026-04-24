import { useState, useEffect, useCallback, useRef } from 'react'

const FONT = "-apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif"

interface Settings {
  agent?: { runtime?: string; args?: string }
  auth?: { method?: string; password?: string }
  rate_limit?: { rpm?: number }
  network?: { block_ips?: string[] }
  discord?: { bot_token?: string; channel_id?: string; allowed_user_ids?: string[]; acp_enabled?: boolean; acp_executable?: string }
  telegram?: { bot_token?: string; chat_id?: string }
  gitlab?: { url?: string; client_id?: string; client_secret?: string; redirect_uri?: string; allowed_ids?: string[]; admin_ids?: string[] }
  workspace?: { sync_enabled?: boolean; sync_interval?: string; git_token?: string; notify_channel?: string; sync_submodules?: boolean }
  log?: { format?: string }
}

interface SettingsResponse {
  settings: Settings
  restart_required: boolean
}

type Tab = 'general' | 'auth' | 'integrations' | 'workspace' | 'advanced'

// ── Custom form controls ──────────────────────────────────────────────

function Checkbox({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', userSelect: 'none' as const }}>
      <div
        onClick={() => onChange(!checked)}
        style={{
          width: 16, height: 16, borderRadius: 4,
          border: checked ? 'none' : '1.5px solid #484848',
          background: checked ? '#4a9eff' : '#1e1e1e',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          flexShrink: 0,
        }}
      >
        {checked && (
          <svg width="10" height="8" viewBox="0 0 10 8" fill="none">
            <polyline points="1,4 3.5,7 9,1" stroke="#fff" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
          </svg>
        )}
      </div>
      <span style={{ fontSize: 13, color: '#c8c8c8', fontFamily: FONT }}>{label}</span>
    </label>
  )
}

function RadioGroup({ options, value, onChange }: { options: string[]; value: string; onChange: (v: string) => void }) {
  return (
    <div style={{ display: 'flex', gap: 20, flexWrap: 'wrap' as const }}>
      {options.map(opt => (
        <label key={opt} onClick={() => onChange(opt)} style={{ display: 'flex', alignItems: 'center', gap: 7, cursor: 'pointer', userSelect: 'none' as const }}>
          <div style={{
            width: 16, height: 16, borderRadius: '50%',
            border: `1.5px solid ${value === opt ? '#4a9eff' : '#484848'}`,
            background: '#1e1e1e',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            flexShrink: 0,
          }}>
            {value === opt && <div style={{ width: 8, height: 8, borderRadius: '50%', background: '#4a9eff' }} />}
          </div>
          <span style={{ fontSize: 13, color: '#c8c8c8', fontFamily: FONT }}>{opt}</span>
        </label>
      ))}
    </div>
  )
}

function Field({ label, children, restartRequired }: { label: string; children: React.ReactNode; restartRequired?: boolean }) {
  return (
    <div style={{ marginBottom: 18 }}>
      <label style={{ display: 'block', fontSize: 12, color: '#7a7a7a', fontFamily: FONT, marginBottom: 6, fontWeight: 500 }}>
        {label}{restartRequired && <span style={{ color: '#c06000', fontSize: 11, marginLeft: 6 }}>⚠ restart</span>}
      </label>
      {children}
    </div>
  )
}

function TextInput({ value, onChange, placeholder, sensitive }: { value: string; onChange: (v: string) => void; placeholder?: string; sensitive?: boolean }) {
  return (
    <input
      type={sensitive ? 'password' : 'text'}
      value={value}
      onChange={e => onChange(e.target.value)}
      placeholder={placeholder}
      style={{
        width: '100%', background: '#1a1a1a', color: '#e0e0e0',
        border: '1px solid #2e2e2e', borderRadius: 7, padding: '8px 10px',
        fontSize: 13, fontFamily: FONT, boxSizing: 'border-box' as const, outline: 'none',
      }}
    />
  )
}

// ── Tab components — expose delta via registerDelta ───────────────────

interface TabProps {
  settings: Settings
  registerDelta: (fn: () => Settings) => void
}

function GeneralTab({ settings, registerDelta }: TabProps) {
  const [runtime, setRuntime] = useState(settings.agent?.runtime ?? '')
  const [args, setArgs] = useState(settings.agent?.args ?? '')
  const [rpm, setRpm] = useState(String(settings.rate_limit?.rpm ?? ''))
  const [blockIPs, setBlockIPs] = useState((settings.network?.block_ips ?? []).join('\n'))

  useEffect(() => {
    setRuntime(settings.agent?.runtime ?? '')
    setArgs(settings.agent?.args ?? '')
    setRpm(String(settings.rate_limit?.rpm ?? ''))
    setBlockIPs((settings.network?.block_ips ?? []).join('\n'))
  }, [settings])

  useEffect(() => {
    registerDelta(() => ({
      agent: { runtime: runtime || undefined, args: args || undefined },
      rate_limit: { rpm: rpm ? Number(rpm) : undefined },
      network: { block_ips: blockIPs.split('\n').map(s => s.trim()).filter(Boolean) },
    }))
  }, [runtime, args, rpm, blockIPs, registerDelta])

  return (
    <>
      <Field label="Agent Runtime" restartRequired>
        <RadioGroup options={['claude', 'opencode']} value={runtime} onChange={setRuntime} />
      </Field>
      <Field label="Agent Args">
        <TextInput value={args} onChange={setArgs} placeholder="--dangerously-skip-permissions" />
      </Field>
      <Field label="Rate Limit (RPM)">
        <TextInput value={rpm} onChange={setRpm} placeholder="10" />
      </Field>
      <Field label="Block IPs (one per line)">
        <textarea
          value={blockIPs}
          onChange={e => setBlockIPs(e.target.value)}
          rows={3}
          style={{ width: '100%', background: '#1a1a1a', color: '#e0e0e0', border: '1px solid #2e2e2e', borderRadius: 7, padding: '8px 10px', fontSize: 13, fontFamily: FONT, boxSizing: 'border-box' as const, resize: 'vertical' as const, outline: 'none' }}
        />
      </Field>
    </>
  )
}

function AuthTab({ settings, registerDelta }: TabProps) {
  const [method, setMethod] = useState(settings.auth?.method ?? '')
  const [password, setPassword] = useState(settings.auth?.password ?? '')

  useEffect(() => {
    setMethod(settings.auth?.method ?? '')
    setPassword(settings.auth?.password ?? '')
  }, [settings])

  useEffect(() => {
    registerDelta(() => ({ auth: { method: method || undefined, password: password || undefined } }))
  }, [method, password, registerDelta])

  return (
    <>
      <Field label="Auth Method" restartRequired>
        <RadioGroup options={['none', 'password', 'gitlab']} value={method} onChange={setMethod} />
      </Field>
      <Field label="Password">
        <TextInput value={password} onChange={setPassword} sensitive placeholder="Enter new password" />
      </Field>
    </>
  )
}

function IntegrationsTab({ settings, registerDelta }: TabProps) {
  const d = settings.discord ?? {}
  const t = settings.telegram ?? {}
  const [discordToken, setDiscordToken] = useState(d.bot_token ?? '')
  const [channelID, setChannelID] = useState(d.channel_id ?? '')
  const [allowedUsers, setAllowedUsers] = useState((d.allowed_user_ids ?? []).join(','))
  const [telegramToken, setTelegramToken] = useState(t.bot_token ?? '')
  const [telegramChat, setTelegramChat] = useState(t.chat_id ?? '')

  useEffect(() => {
    const disc = settings.discord ?? {}
    const tele = settings.telegram ?? {}
    setDiscordToken(disc.bot_token ?? '')
    setChannelID(disc.channel_id ?? '')
    setAllowedUsers((disc.allowed_user_ids ?? []).join(','))
    setTelegramToken(tele.bot_token ?? '')
    setTelegramChat(tele.chat_id ?? '')
  }, [settings])

  useEffect(() => {
    registerDelta(() => ({
      discord: { bot_token: discordToken || undefined, channel_id: channelID || undefined, allowed_user_ids: allowedUsers.split(',').map(s => s.trim()).filter(Boolean) },
      telegram: { bot_token: telegramToken || undefined, chat_id: telegramChat || undefined },
    }))
  }, [discordToken, channelID, allowedUsers, telegramToken, telegramChat, registerDelta])

  const sectionHead = (text: string, first = false) => (
    <div style={{ color: '#666', fontSize: 11, fontFamily: FONT, fontWeight: 600, textTransform: 'uppercase' as const, letterSpacing: '0.06em', marginBottom: 14, marginTop: first ? 0 : 20, paddingTop: first ? 0 : 16, borderTop: first ? 'none' : '1px solid #1e1e1e' }}>{text}</div>
  )

  return (
    <>
      {sectionHead('Discord', true)}
      <Field label="Bot Token" restartRequired><TextInput value={discordToken} onChange={setDiscordToken} sensitive /></Field>
      <Field label="Channel ID" restartRequired><TextInput value={channelID} onChange={setChannelID} /></Field>
      <Field label="Allowed DM User IDs (comma-separated)">
        <TextInput value={allowedUsers} onChange={setAllowedUsers} placeholder="123,456" />
      </Field>
      {sectionHead('Telegram')}
      <Field label="Bot Token" restartRequired><TextInput value={telegramToken} onChange={setTelegramToken} sensitive /></Field>
      <Field label="Chat ID" restartRequired><TextInput value={telegramChat} onChange={setTelegramChat} /></Field>
    </>
  )
}

function WorkspaceTab({ settings, registerDelta }: TabProps) {
  const w = settings.workspace ?? {}
  const [enabled, setEnabled] = useState(w.sync_enabled ?? false)
  const [syncInterval, setSyncInterval] = useState(w.sync_interval ?? '60s')
  const [token, setToken] = useState(w.git_token ?? '')
  const [notify, setNotify] = useState(w.notify_channel ?? '')
  const [submodules, setSubmodules] = useState(w.sync_submodules ?? false)

  useEffect(() => {
    const ws = settings.workspace ?? {}
    setEnabled(ws.sync_enabled ?? false)
    setSyncInterval(ws.sync_interval ?? '60s')
    setToken(ws.git_token ?? '')
    setNotify(ws.notify_channel ?? '')
    setSubmodules(ws.sync_submodules ?? false)
  }, [settings])

  useEffect(() => {
    registerDelta(() => ({ workspace: { sync_enabled: enabled, sync_interval: syncInterval || undefined, git_token: token || undefined, notify_channel: notify || undefined, sync_submodules: submodules } }))
  }, [enabled, syncInterval, token, notify, submodules, registerDelta])

  return (
    <>
      <Field label="Git Sync">
        <Checkbox checked={enabled} onChange={setEnabled} label="Enable workspace git sync" />
      </Field>
      <Field label="Sync Interval"><TextInput value={syncInterval} onChange={setSyncInterval} placeholder="60s" /></Field>
      <Field label="Git Token"><TextInput value={token} onChange={setToken} sensitive /></Field>
      <Field label="Notify Discord Channel"><TextInput value={notify} onChange={setNotify} /></Field>
      <Field label="Submodules">
        <Checkbox checked={submodules} onChange={setSubmodules} label="Include submodules" />
      </Field>
    </>
  )
}

function AdvancedTab({ settings, registerDelta }: TabProps) {
  const [logFormat, setLogFormat] = useState(settings.log?.format ?? '')

  useEffect(() => { setLogFormat(settings.log?.format ?? '') }, [settings])

  useEffect(() => {
    registerDelta(() => ({ log: { format: logFormat || undefined } }))
  }, [logFormat, registerDelta])

  return (
    <Field label="Log Format">
      <RadioGroup options={['text', 'json']} value={logFormat} onChange={setLogFormat} />
    </Field>
  )
}

// ── Main dialog ───────────────────────────────────────────────────────

export default function SettingsPanel() {
  const [open, setOpen] = useState(false)
  const [tab, setTab] = useState<Tab>('general')
  const [settings, setSettings] = useState<Settings>({})
  const [restartRequired, setRestartRequired] = useState(false)
  const [saving, setSaving] = useState(false)
  const [restarting, setRestarting] = useState(false)
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null)
  const getDeltaRef = useRef<() => Settings>(() => ({}))

  const registerDelta = useCallback((fn: () => Settings) => {
    getDeltaRef.current = fn
  }, [])

  const load = useCallback(async () => {
    try {
      const r = await fetch('/api/settings')
      if (r.ok) {
        const data: SettingsResponse = await r.json()
        setSettings(data.settings)
        setRestartRequired(data.restart_required)
      }
    } catch { /* ignore */ }
  }, [])

  useEffect(() => {
    const handler = () => { setOpen(true); load() }
    window.addEventListener('perch:open-settings', handler)
    return () => window.removeEventListener('perch:open-settings', handler)
  }, [load])

  const doSave = async (): Promise<boolean> => {
    setSaving(true)
    try {
      const r = await fetch('/api/settings', {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(getDeltaRef.current()),
      })
      if (r.ok) {
        const data: SettingsResponse = await r.json()
        setSettings(data.settings)
        setRestartRequired(data.restart_required)
        return data.restart_required
      }
      setToast({ msg: 'Save failed', ok: false })
      return false
    } catch {
      setToast({ msg: 'Network error', ok: false })
      return false
    } finally {
      setSaving(false)
    }
  }

  const handleSave = async () => {
    const needsRestart = await doSave()
    if (!needsRestart) {
      setToast({ msg: 'Saved', ok: true })
      setTimeout(() => setToast(null), 3000)
    }
    // If needsRestart is now true, button text switches to "Save & Restart" automatically
  }

  const handleSaveAndRestart = async () => {
    if (!window.confirm('Save and restart the container? The page will reload when the server is back up.')) return
    await doSave()
    setRestarting(true)
    setToast({ msg: 'Restarting…', ok: true })
    try { await fetch('/api/admin/restart', { method: 'POST' }) } catch { /* may close before responding */ }

    // Wait for server to go down, then poll until back
    await new Promise(r => setTimeout(r, 3000))
    for (let i = 0; i < 30; i++) {
      await new Promise(r => setTimeout(r, 2000))
      try {
        const r = await fetch('/api/auth/status')
        if (r.ok) { window.location.href = '/chat'; return }
      } catch { /* still down */ }
    }
    setToast({ msg: 'Server did not come back after 60s', ok: false })
    setRestarting(false)
  }

  if (!open) return null

  const TABS: { id: Tab; label: string }[] = [
    { id: 'general', label: 'General' },
    { id: 'auth', label: 'Auth' },
    { id: 'integrations', label: 'Integrations' },
    { id: 'workspace', label: 'Workspace' },
    { id: 'advanced', label: 'Advanced' },
  ]

  const tabProps = { settings, registerDelta }
  const busy = saving || restarting
  const btnLabel = busy ? (restarting ? 'Restarting…' : 'Saving…') : restartRequired ? 'Save & Restart' : 'Save'

  return (
    <>
      <style>{`
        .perch-scroll::-webkit-scrollbar { width: 5px; }
        .perch-scroll::-webkit-scrollbar-track { background: transparent; }
        .perch-scroll::-webkit-scrollbar-thumb { background: #2e2e2e; border-radius: 3px; }
        .perch-scroll::-webkit-scrollbar-thumb:hover { background: #444; }
      `}</style>
      {/* Backdrop */}
      <div onClick={() => setOpen(false)} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.65)', zIndex: 200 }} />
      {/* Dialog — fixed size so tab switches don't resize it */}
      <div style={{
        position: 'fixed', left: '50%', top: '50%', transform: 'translate(-50%, -50%)',
        width: 'min(640px, 92vw)', height: 'min(600px, 85vh)',
        background: '#161616', border: '1px solid #2a2a2a', borderRadius: 14,
        zIndex: 201, display: 'flex', flexDirection: 'column', fontFamily: FONT,
        boxShadow: '0 24px 60px rgba(0,0,0,0.7)',
      }}>
        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '18px 22px 0', flexShrink: 0 }}>
          <span style={{ color: '#e8e8e8', fontSize: 16, fontWeight: 600, letterSpacing: '-0.02em' }}>Settings</span>
          <button onClick={() => setOpen(false)}
            style={{ background: 'none', border: 'none', color: '#606060', cursor: 'pointer', fontSize: 20, lineHeight: 1, padding: '4px 8px', borderRadius: 6 }}
            onMouseEnter={e => (e.currentTarget.style.color = '#c0c0c0')}
            onMouseLeave={e => (e.currentTarget.style.color = '#606060')}
          >×</button>
        </div>

        {/* Tabs */}
        <div style={{ display: 'flex', borderBottom: '1px solid #252525', overflowX: 'auto', padding: '4px 18px 0', marginTop: 4, flexShrink: 0 }}>
          {TABS.map(t => (
            <button key={t.id} onClick={() => setTab(t.id)} style={{
              padding: '8px 14px', border: 'none', background: 'none',
              color: tab === t.id ? '#e0e0e0' : '#666',
              borderBottom: tab === t.id ? '2px solid #4a9eff' : '2px solid transparent',
              cursor: 'pointer', fontSize: 13, fontFamily: FONT, whiteSpace: 'nowrap',
              fontWeight: tab === t.id ? 500 : 400, transition: 'color 0.12s',
            }}>{t.label}</button>
          ))}
        </div>

        {/* Content — scrollable, fixed height */}
        <div className="perch-scroll" style={{ flex: 1, overflowY: 'auto', padding: '20px 22px 8px', minHeight: 0 }}>
          {tab === 'general' && <GeneralTab {...tabProps} />}
          {tab === 'auth' && <AuthTab {...tabProps} />}
          {tab === 'integrations' && <IntegrationsTab {...tabProps} />}
          {tab === 'workspace' && <WorkspaceTab {...tabProps} />}
          {tab === 'advanced' && <AdvancedTab {...tabProps} />}
        </div>

        {/* Footer */}
        <div style={{ borderTop: '1px solid #222', padding: '14px 22px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexShrink: 0 }}>
          <div style={{ fontSize: 12, color: toast ? (toast.ok ? '#4a9eff' : '#e05555') : 'transparent', minHeight: 16 }}>
            {toast?.msg ?? ''}
          </div>
          <button
            onClick={restartRequired ? handleSaveAndRestart : handleSave}
            disabled={busy}
            style={{
              background: busy ? '#222' : restartRequired ? 'rgba(224,112,48,0.15)' : '#4a9eff',
              border: restartRequired && !busy ? '1px solid #a04010' : 'none',
              color: busy ? '#555' : restartRequired ? '#d06020' : '#fff',
              borderRadius: 8, padding: '9px 22px',
              cursor: busy ? 'not-allowed' : 'pointer',
              fontSize: 13, fontFamily: FONT, fontWeight: 500,
              minWidth: 120, transition: 'background 0.15s',
            }}
          >{btnLabel}</button>
        </div>
      </div>
    </>
  )
}
