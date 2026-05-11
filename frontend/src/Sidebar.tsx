import type { MouseEvent } from 'react'
import { useState } from 'react'
import ConversationList from './ConversationList'

const FONT = "-apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif"

interface SidebarProps {
  isAdmin: boolean
  authMethod: string
  authenticated?: boolean
  accessDenied?: boolean
  onNewChat: () => void
  onCollapse: () => void
  activeConversationId?: string
  onSelectConversation?: (id: string) => void
}

export default function Sidebar({ isAdmin, authMethod, authenticated = true, accessDenied, onNewChat, onCollapse, activeConversationId, onSelectConversation }: SidebarProps) {
  return (
    <div style={{
      width: 260, flexShrink: 0, display: 'flex', flexDirection: 'column',
      background: '#111', borderRight: '1px solid #252525', height: '100dvh',
      fontFamily: FONT,
    }}>
      {/* Logo + collapse */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '14px 14px 10px' }}>
        <div style={{ fontSize: 15, color: '#e0e0e0', fontWeight: 600, letterSpacing: '-0.01em' }}>
          Perch
        </div>
        <button
          onClick={onCollapse}
          title="Collapse sidebar"
          style={{ background: 'none', border: 'none', color: '#555', cursor: 'pointer', padding: 6, borderRadius: 6, display: 'flex', alignItems: 'center', justifyContent: 'center' }}
          onMouseEnter={e => (e.currentTarget.style.background = 'rgba(255,255,255,0.06)')}
          onMouseLeave={e => (e.currentTarget.style.background = 'none')}
        >
          <SidebarCollapseIcon />
        </button>
      </div>

      {authenticated ? (
        <>
          {/* New Chat */}
          <div style={{ padding: '2px 10px 10px' }}>
            <button
              onClick={onNewChat}
              style={{
                width: '100%', background: 'rgba(255,255,255,0.05)', border: '1px solid #2e2e2e',
                borderRadius: 8, color: '#c8c8c8', padding: '8px 12px',
                fontSize: 13, cursor: 'pointer', fontFamily: FONT,
                display: 'flex', alignItems: 'center', gap: 8,
                transition: 'background 0.15s',
              }}
              onMouseEnter={e => (e.currentTarget.style.background = 'rgba(255,255,255,0.09)')}
              onMouseLeave={e => (e.currentTarget.style.background = 'rgba(255,255,255,0.05)')}
            >
              <PlusIcon />
              New Chat
            </button>
          </div>

          {/* Conversation list */}
          <div style={{ flex: 1, overflowY: 'auto' }}>
            <ConversationList activeId={activeConversationId} onSelect={onSelectConversation} />
          </div>

          {/* Bottom nav */}
          <div style={{ borderTop: '1px solid #222', padding: '6px 8px 10px' }}>
            {isAdmin && (
              <>
                <NavItem
                  href="/chat"
                  icon={<SettingsIcon />}
                  label="Settings"
                  onClick={(e) => {
                    e.preventDefault()
                    window.dispatchEvent(new CustomEvent('perch:open-settings'))
                  }}
                />
                <NavItem href="/terminal" icon={<TerminalIcon />} label="Terminal" />
                <NavItem href="/admin" icon={<ShieldIcon />} label="Admin" />
              </>
            )}
            {authMethod !== 'none' && (
              <NavItem href="/auth/logout" icon={<LogoutIcon />} label="Log out" />
            )}
          </div>
        </>
      ) : (
        /* Unauthenticated GitLab: show login button in sidebar */
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', padding: '24px 16px', gap: 12 }}>
          {accessDenied && (
            <div style={{
              color: '#f66', fontSize: 12, textAlign: 'center', padding: '8px 12px',
              background: '#2a0000', borderRadius: 6, border: '1px solid #800', width: '100%',
            }}>
              Access denied. Contact the administrator.
            </div>
          )}
          <button
            onClick={() => { window.location.href = '/auth/gitlab' }}
            style={{
              width: '100%', background: '#fc6d26', border: 'none', borderRadius: 6,
              color: '#fff', padding: '10px 16px', fontSize: 13,
              cursor: 'pointer', fontFamily: FONT, fontWeight: 600,
            }}
          >
            Login with GitLab
          </button>
        </div>
      )}
    </div>
  )
}

function NavItem({ href, icon, label, onClick }: { href: string; icon: React.ReactNode; label: string; onClick?: (e: MouseEvent<HTMLAnchorElement>) => void }) {
  const [hovered, setHovered] = useState(false)
  return (
    <a
      href={href}
      onClick={onClick}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      style={{
        display: 'flex', alignItems: 'center', gap: 10,
        padding: '8px 10px', color: hovered ? '#e0e0e0' : '#a8a8a8',
        textDecoration: 'none', fontSize: 13, fontFamily: FONT,
        borderRadius: 7, cursor: 'pointer',
        background: hovered ? 'rgba(255,255,255,0.06)' : 'transparent',
        transition: 'background 0.12s, color 0.12s',
        marginBottom: 1,
      }}
    >
      <span style={{ flexShrink: 0, display: 'flex', alignItems: 'center', color: hovered ? '#c0c0c0' : '#707070' }}>
        {icon}
      </span>
      {label}
    </a>
  )
}

// ── Minimal stroke SVG icons (Feather-style) ──────────────────────────

function SidebarCollapseIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="3" width="18" height="18" rx="2"/>
      <line x1="9" y1="3" x2="9" y2="21"/>
      <polyline points="14 9 11 12 14 15"/>
    </svg>
  )
}

function PlusIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
      <line x1="12" y1="5" x2="12" y2="19"/>
      <line x1="5" y1="12" x2="19" y2="12"/>
    </svg>
  )
}

function SettingsIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3"/>
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
    </svg>
  )
}

function TerminalIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="4 17 10 11 4 5"/>
      <line x1="12" y1="19" x2="20" y2="19"/>
    </svg>
  )
}

function ShieldIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>
    </svg>
  )
}

function LogoutIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>
      <polyline points="16 17 21 12 16 7"/>
      <line x1="21" y1="12" x2="9" y2="12"/>
    </svg>
  )
}
