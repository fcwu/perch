import { useState, useEffect, useCallback } from 'react'

const FONT = "-apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif"

// Settings shape from API
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

export default function SettingsPanel() {
  const [open, setOpen] = useState(false)
  const [tab, setTab] = useState<Tab>('general')
  const [settings, setSettings] = useState<Settings>({})
  const [restartRequired, setRestartRequired] = useState(false)
  const [saving, setSaving] = useState(false)
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null)

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

  const save = async (delta: Settings) => {
    setSaving(true)
    try {
      const r = await fetch('/api/settings', {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(delta),
      })
      if (r.ok) {
        const data: SettingsResponse = await r.json()
        setSettings(data.settings)
        setRestartRequired(data.restart_required)
        setToast({ msg: 'Saved', ok: true })
      } else {
        setToast({ msg: 'Save failed', ok: false })
      }
    } catch {
      setToast({ msg: 'Network error', ok: false })
    } finally {
      setSaving(false)
      setTimeout(() => setToast(null), 3000)
    }
  }

  const handleRestart = async () => {
    if (!window.confirm('Restart the container? The server will be temporarily unavailable.')) return
    setToast({ msg: 'Restarting…', ok: true })
    await fetch('/api/admin/restart', { method: 'POST' })
  }

  if (!open) return null

  // Tab content
  const renderTabContent = () => {
    switch (tab) {
      case 'general': return (
        <GeneralTab settings={settings} onSave={save} saving={saving} />
      )
      case 'auth': return (
        <AuthTab settings={settings} onSave={save} saving={saving} />
      )
      case 'integrations': return (
        <IntegrationsTab settings={settings} onSave={save} saving={saving} />
      )
      case 'workspace': return (
        <WorkspaceTab settings={settings} onSave={save} saving={saving} />
      )
      case 'advanced': return (
        <AdvancedTab settings={settings} onSave={save} saving={saving} />
      )
    }
  }

  const TABS: { id: Tab; label: string }[] = [
    { id: 'general', label: 'General' },
    { id: 'auth', label: 'Auth' },
    { id: 'integrations', label: 'Integrations' },
    { id: 'workspace', label: 'Workspace' },
    { id: 'advanced', label: 'Advanced' },
  ]

  return (
    <>
      {/* Backdrop */}
      <div
        onClick={() => setOpen(false)}
        style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', zIndex: 200 }}
      />
      {/* Panel */}
      <div style={{
        position: 'fixed', left: 0, top: 0, bottom: 0, width: 380,
        background: '#111', borderRight: '1px solid #252525', zIndex: 201,
        display: 'flex', flexDirection: 'column', fontFamily: FONT,
      }}>
        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px 18px', borderBottom: '1px solid #222' }}>
          <span style={{ color: '#e0e0e0', fontSize: 15, fontWeight: 600, letterSpacing: '-0.01em' }}>Settings</span>
          <button onClick={() => setOpen(false)}
            style={{ background: 'none', border: 'none', color: '#555', cursor: 'pointer', fontSize: 20, lineHeight: 1, padding: '4px 6px', borderRadius: 6 }}
            onMouseEnter={e => (e.currentTarget.style.background = 'rgba(255,255,255,0.06)')}
            onMouseLeave={e => (e.currentTarget.style.background = 'none')}
          >×</button>
        </div>

        {/* Tabs */}
        <div style={{ display: 'flex', borderBottom: '1px solid #222', overflowX: 'auto', padding: '0 4px' }}>
          {TABS.map(t => (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              style={{
                padding: '10px 12px', border: 'none', background: 'none',
                color: tab === t.id ? '#e0e0e0' : '#5a5a5a',
                borderBottom: tab === t.id ? '2px solid #4a9eff' : '2px solid transparent',
                cursor: 'pointer', fontSize: 12, fontFamily: FONT, whiteSpace: 'nowrap',
                fontWeight: tab === t.id ? 500 : 400,
                transition: 'color 0.12s',
              }}
            >{t.label}</button>
          ))}
        </div>

        {/* Tab content */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '20px 18px' }}>
          {renderTabContent()}
        </div>

        {/* Footer */}
        <div style={{ borderTop: '1px solid #222', padding: '12px 18px', display: 'flex', flexDirection: 'column', gap: 8 }}>
          {toast && (
            <div style={{ fontSize: 12, color: toast.ok ? '#4a9eff' : '#e05555', textAlign: 'center' }}>
              {toast.msg}
            </div>
          )}
          <button
            onClick={handleRestart}
            style={{
              background: restartRequired ? 'rgba(255,107,0,0.1)' : 'rgba(255,255,255,0.04)',
              border: `1px solid ${restartRequired ? '#ff6b00' : '#2e2e2e'}`,
              color: restartRequired ? '#ff6b00' : '#5a5a5a',
              borderRadius: 7, padding: '8px 14px', cursor: 'pointer',
              fontSize: 12, fontFamily: FONT, fontWeight: 500,
            }}
            onMouseEnter={e => (e.currentTarget.style.background = restartRequired ? 'rgba(255,107,0,0.18)' : 'rgba(255,255,255,0.07)')}
            onMouseLeave={e => (e.currentTarget.style.background = restartRequired ? 'rgba(255,107,0,0.1)' : 'rgba(255,255,255,0.04)')}
          >
            {restartRequired ? '⚠ Restart Container' : 'Restart Container'}
          </button>
        </div>
      </div>
    </>
  )
}

// Reusable field wrapper
function Field({ label, children, restartRequired }: { label: string; children: React.ReactNode; restartRequired?: boolean }) {
  return (
    <div style={{ marginBottom: 18 }}>
      <label style={{ display: 'block', fontSize: 12, color: '#7a7a7a', fontFamily: FONT, marginBottom: 6, fontWeight: 500 }}>
        {label} {restartRequired && <span style={{ color: '#c06000', fontSize: 11 }}>⚠ restart</span>}
      </label>
      {children}
    </div>
  )
}

// Reusable text input
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
        fontSize: 13, fontFamily: FONT, boxSizing: 'border-box',
        outline: 'none',
      }}
    />
  )
}

// Save button
function SaveButton({ onClick, saving }: { onClick: () => void; saving: boolean }) {
  return (
    <button
      onClick={onClick}
      disabled={saving}
      style={{
        background: saving ? '#2a3a4a' : '#4a9eff', border: 'none', color: '#fff',
        borderRadius: 7, padding: '8px 20px', cursor: saving ? 'not-allowed' : 'pointer',
        fontSize: 13, fontFamily: FONT, fontWeight: 500,
      }}
    >{saving ? 'Saving…' : 'Save'}</button>
  )
}

interface TabProps { settings: Settings; onSave: (delta: Settings) => Promise<void>; saving: boolean }

function GeneralTab({ settings, onSave, saving }: TabProps) {
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

  return (
    <>
      <Field label="Agent Runtime" restartRequired>
        <div style={{ display: 'flex', gap: 8 }}>
          {['claude', 'opencode'].map(r => (
            <label key={r} style={{ display: 'flex', alignItems: 'center', gap: 4, color: '#aaa', fontSize: 13, fontFamily: FONT, cursor: 'pointer' }}>
              <input type="radio" checked={runtime === r} onChange={() => setRuntime(r)} /> {r}
            </label>
          ))}
        </div>
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
          style={{ width: '100%', background: '#1a1a1a', color: '#e0e0e0', border: '1px solid #2e2e2e', borderRadius: 7, padding: '8px 10px', fontSize: 13, fontFamily: FONT, boxSizing: 'border-box', resize: 'vertical', outline: 'none' }}
        />
      </Field>
      <SaveButton saving={saving} onClick={() => onSave({
        agent: { runtime: runtime || undefined, args: args || undefined },
        rate_limit: { rpm: rpm ? Number(rpm) : undefined },
        network: { block_ips: blockIPs.split('\n').map(s => s.trim()).filter(Boolean) },
      })} />
    </>
  )
}

function AuthTab({ settings, onSave, saving }: TabProps) {
  const [method, setMethod] = useState(settings.auth?.method ?? '')
  const [password, setPassword] = useState(settings.auth?.password ?? '')

  useEffect(() => {
    setMethod(settings.auth?.method ?? '')
    setPassword(settings.auth?.password ?? '')
  }, [settings])

  return (
    <>
      <Field label="Auth Method" restartRequired>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          {['none', 'password', 'gitlab'].map(m => (
            <label key={m} style={{ display: 'flex', alignItems: 'center', gap: 4, color: '#aaa', fontSize: 13, fontFamily: FONT, cursor: 'pointer' }}>
              <input type="radio" checked={method === m} onChange={() => setMethod(m)} /> {m}
            </label>
          ))}
        </div>
      </Field>
      <Field label="Password">
        <TextInput value={password} onChange={setPassword} sensitive placeholder="Enter new password" />
      </Field>
      <SaveButton saving={saving} onClick={() => onSave({ auth: { method: method || undefined, password: password || undefined } })} />
    </>
  )
}

function IntegrationsTab({ settings, onSave, saving }: TabProps) {
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

  return (
    <>
      <div style={{ color: '#8a8a8a', fontSize: 11, fontFamily: FONT, marginBottom: 14, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em' }}>Discord</div>
      <Field label="Bot Token" restartRequired><TextInput value={discordToken} onChange={setDiscordToken} sensitive /></Field>
      <Field label="Channel ID" restartRequired><TextInput value={channelID} onChange={setChannelID} /></Field>
      <Field label="Allowed DM User IDs (comma-separated)">
        <TextInput value={allowedUsers} onChange={setAllowedUsers} placeholder="123,456" />
      </Field>
      <div style={{ color: '#8a8a8a', fontSize: 11, fontFamily: FONT, margin: '20px 0 14px', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em', borderTop: '1px solid #1e1e1e', paddingTop: 16 }}>Telegram</div>
      <Field label="Bot Token" restartRequired><TextInput value={telegramToken} onChange={setTelegramToken} sensitive /></Field>
      <Field label="Chat ID" restartRequired><TextInput value={telegramChat} onChange={setTelegramChat} /></Field>
      <SaveButton saving={saving} onClick={() => onSave({
        discord: { bot_token: discordToken || undefined, channel_id: channelID || undefined, allowed_user_ids: allowedUsers.split(',').map(s => s.trim()).filter(Boolean) },
        telegram: { bot_token: telegramToken || undefined, chat_id: telegramChat || undefined },
      })} />
    </>
  )
}

function WorkspaceTab({ settings, onSave, saving }: TabProps) {
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

  return (
    <>
      <Field label="Git Sync Enabled">
        <label style={{ display: 'flex', alignItems: 'center', gap: 8, color: '#aaa', fontSize: 13, fontFamily: FONT, cursor: 'pointer' }}>
          <input type="checkbox" checked={enabled} onChange={e => setEnabled(e.target.checked)} /> Enable workspace git sync
        </label>
      </Field>
      <Field label="Sync Interval"><TextInput value={syncInterval} onChange={setSyncInterval} placeholder="60s" /></Field>
      <Field label="Git Token"><TextInput value={token} onChange={setToken} sensitive /></Field>
      <Field label="Notify Discord Channel"><TextInput value={notify} onChange={setNotify} /></Field>
      <Field label="Sync Submodules">
        <label style={{ display: 'flex', alignItems: 'center', gap: 8, color: '#aaa', fontSize: 13, fontFamily: FONT, cursor: 'pointer' }}>
          <input type="checkbox" checked={submodules} onChange={e => setSubmodules(e.target.checked)} /> Include submodules
        </label>
      </Field>
      <SaveButton saving={saving} onClick={() => onSave({ workspace: { sync_enabled: enabled, sync_interval: syncInterval || undefined, git_token: token || undefined, notify_channel: notify || undefined, sync_submodules: submodules } })} />
    </>
  )
}

function AdvancedTab({ settings, onSave, saving }: TabProps) {
  const [logFormat, setLogFormat] = useState(settings.log?.format ?? '')

  useEffect(() => {
    setLogFormat(settings.log?.format ?? '')
  }, [settings])

  return (
    <>
      <Field label="Log Format">
        <div style={{ display: 'flex', gap: 8 }}>
          {['text', 'json'].map(f => (
            <label key={f} style={{ display: 'flex', alignItems: 'center', gap: 4, color: '#aaa', fontSize: 13, fontFamily: FONT, cursor: 'pointer' }}>
              <input type="radio" checked={logFormat === f} onChange={() => setLogFormat(f)} /> {f}
            </label>
          ))}
        </div>
      </Field>
      <SaveButton saving={saving} onClick={() => onSave({ log: { format: logFormat || undefined } })} />
    </>
  )
}
