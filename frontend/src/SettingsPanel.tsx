import { useState, useEffect, useCallback } from 'react'

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
        position: 'fixed', left: 0, top: 0, bottom: 0, width: 360,
        background: '#111', borderRight: '1px solid #222', zIndex: 201,
        display: 'flex', flexDirection: 'column',
      }}>
        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px', borderBottom: '1px solid #222' }}>
          <span style={{ color: '#e0e0e0', fontFamily: 'sans-serif', fontSize: 15, fontWeight: 600 }}>Settings</span>
          <button onClick={() => setOpen(false)} style={{ background: 'none', border: 'none', color: '#666', cursor: 'pointer', fontSize: 18 }}>×</button>
        </div>

        {/* Tabs */}
        <div style={{ display: 'flex', borderBottom: '1px solid #222', overflowX: 'auto' }}>
          {TABS.map(t => (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              style={{
                padding: '8px 12px', border: 'none', background: 'none',
                color: tab === t.id ? '#4a9eff' : '#666',
                borderBottom: tab === t.id ? '2px solid #4a9eff' : '2px solid transparent',
                cursor: 'pointer', fontSize: 12, fontFamily: 'sans-serif', whiteSpace: 'nowrap',
              }}
            >{t.label}</button>
          ))}
        </div>

        {/* Tab content */}
        <div style={{ flex: 1, overflowY: 'auto', padding: 16 }}>
          {renderTabContent()}
        </div>

        {/* Footer */}
        <div style={{ borderTop: '1px solid #222', padding: 12, display: 'flex', flexDirection: 'column', gap: 8 }}>
          {toast && (
            <div style={{ fontSize: 12, color: toast.ok ? '#4af' : '#f66', fontFamily: 'sans-serif', textAlign: 'center' }}>
              {toast.msg}
            </div>
          )}
          <button
            onClick={handleRestart}
            style={{
              background: restartRequired ? '#ff6b0020' : 'none',
              border: `1px solid ${restartRequired ? '#ff6b00' : '#444'}`,
              color: restartRequired ? '#ff6b00' : '#666',
              borderRadius: 4, padding: '6px 12px', cursor: 'pointer',
              fontSize: 12, fontFamily: 'sans-serif',
            }}
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
    <div style={{ marginBottom: 16 }}>
      <label style={{ display: 'block', fontSize: 11, color: '#666', fontFamily: 'sans-serif', marginBottom: 4 }}>
        {label} {restartRequired && <span style={{ color: '#ff6b00' }}>⚠ restart required</span>}
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
        border: '1px solid #333', borderRadius: 4, padding: '6px 8px',
        fontSize: 13, fontFamily: 'monospace', boxSizing: 'border-box',
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
        background: '#4a9eff', border: 'none', color: '#000',
        borderRadius: 4, padding: '6px 16px', cursor: saving ? 'not-allowed' : 'pointer',
        fontSize: 13, fontFamily: 'sans-serif',
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

  return (
    <>
      <Field label="Agent Runtime" restartRequired>
        <div style={{ display: 'flex', gap: 8 }}>
          {['claude', 'opencode'].map(r => (
            <label key={r} style={{ display: 'flex', alignItems: 'center', gap: 4, color: '#aaa', fontSize: 13, fontFamily: 'sans-serif', cursor: 'pointer' }}>
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
          style={{ width: '100%', background: '#1a1a1a', color: '#e0e0e0', border: '1px solid #333', borderRadius: 4, padding: '6px 8px', fontSize: 13, fontFamily: 'monospace', boxSizing: 'border-box', resize: 'vertical' }}
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
  return (
    <>
      <Field label="Auth Method" restartRequired>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          {['none', 'password', 'gitlab'].map(m => (
            <label key={m} style={{ display: 'flex', alignItems: 'center', gap: 4, color: '#aaa', fontSize: 13, fontFamily: 'sans-serif', cursor: 'pointer' }}>
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
  return (
    <>
      <div style={{ color: '#4a9eff', fontSize: 12, fontFamily: 'sans-serif', marginBottom: 12, fontWeight: 600 }}>Discord</div>
      <Field label="Bot Token" restartRequired><TextInput value={discordToken} onChange={setDiscordToken} sensitive /></Field>
      <Field label="Channel ID" restartRequired><TextInput value={channelID} onChange={setChannelID} /></Field>
      <Field label="Allowed DM User IDs (comma-separated)">
        <TextInput value={allowedUsers} onChange={setAllowedUsers} placeholder="123,456" />
      </Field>
      <div style={{ color: '#4a9eff', fontSize: 12, fontFamily: 'sans-serif', margin: '16px 0 12px', fontWeight: 600 }}>Telegram</div>
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
  return (
    <>
      <Field label="Git Sync Enabled">
        <label style={{ display: 'flex', alignItems: 'center', gap: 8, color: '#aaa', fontSize: 13, fontFamily: 'sans-serif', cursor: 'pointer' }}>
          <input type="checkbox" checked={enabled} onChange={e => setEnabled(e.target.checked)} /> Enable workspace git sync
        </label>
      </Field>
      <Field label="Sync Interval"><TextInput value={syncInterval} onChange={setSyncInterval} placeholder="60s" /></Field>
      <Field label="Git Token"><TextInput value={token} onChange={setToken} sensitive /></Field>
      <Field label="Notify Discord Channel"><TextInput value={notify} onChange={setNotify} /></Field>
      <Field label="Sync Submodules">
        <label style={{ display: 'flex', alignItems: 'center', gap: 8, color: '#aaa', fontSize: 13, fontFamily: 'sans-serif', cursor: 'pointer' }}>
          <input type="checkbox" checked={submodules} onChange={e => setSubmodules(e.target.checked)} /> Include submodules
        </label>
      </Field>
      <SaveButton saving={saving} onClick={() => onSave({ workspace: { sync_enabled: enabled, sync_interval: syncInterval || undefined, git_token: token || undefined, notify_channel: notify || undefined, sync_submodules: submodules } })} />
    </>
  )
}

function AdvancedTab({ settings, onSave, saving }: TabProps) {
  const [logFormat, setLogFormat] = useState(settings.log?.format ?? '')
  return (
    <>
      <Field label="Log Format">
        <div style={{ display: 'flex', gap: 8 }}>
          {['text', 'json'].map(f => (
            <label key={f} style={{ display: 'flex', alignItems: 'center', gap: 4, color: '#aaa', fontSize: 13, fontFamily: 'sans-serif', cursor: 'pointer' }}>
              <input type="radio" checked={logFormat === f} onChange={() => setLogFormat(f)} /> {f}
            </label>
          ))}
        </div>
      </Field>
      <SaveButton saving={saving} onClick={() => onSave({ log: { format: logFormat || undefined } })} />
    </>
  )
}
