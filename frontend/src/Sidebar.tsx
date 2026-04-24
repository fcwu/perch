import type { MouseEvent } from 'react'
import ConversationList from './ConversationList'

interface SidebarProps {
  isAdmin: boolean
  authMethod: string
  onNewChat: () => void
  onCollapse: () => void
  activeConversationId?: string
}

export default function Sidebar({ isAdmin, authMethod, onNewChat, onCollapse, activeConversationId }: SidebarProps) {
  return (
    <div style={{
      width: 260, flexShrink: 0, display: 'flex', flexDirection: 'column',
      background: '#111', borderRight: '1px solid #222', height: '100dvh',
    }}>
      {/* Logo + collapse button */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px 16px 8px' }}>
        <div style={{ fontFamily: 'monospace', fontSize: 18, color: '#4a9eff', fontWeight: 700 }}>
          🐦 Perch
        </div>
        <button
          onClick={onCollapse}
          style={{ background: 'none', border: 'none', color: '#555', cursor: 'pointer', fontSize: 18, padding: 4 }}
        >×</button>
      </div>

      {/* New Chat */}
      <div style={{ padding: '4px 8px 8px' }}>
        <button
          onClick={onNewChat}
          style={{
            width: '100%', background: '#1a1a1a', border: '1px solid #333',
            borderRadius: 6, color: '#e0e0e0', padding: '8px 12px',
            fontSize: 13, cursor: 'pointer', fontFamily: 'sans-serif',
            textAlign: 'left',
          }}
        >
          + New Chat
        </button>
      </div>

      {/* Conversation list */}
      <div style={{ flex: 1, overflowY: 'auto' }}>
        <ConversationList activeId={activeConversationId} />
      </div>

      {/* Bottom nav */}
      <div style={{ borderTop: '1px solid #222', padding: '8px 4px' }}>
        {isAdmin && (
          <>
            <NavLink href="/chat" label="⚙ Settings" onClick={(e) => {
              e.preventDefault()
              // Settings panel is opened via ChatPage — emit a custom event
              window.dispatchEvent(new CustomEvent('perch:open-settings'))
            }} />
            <NavLink href="/terminal" label="🖥 Terminal" />
            <NavLink href="/admin" label="🛡 Admin" />
          </>
        )}
        {authMethod !== 'none' && (
          <NavLink href="/auth/logout" label="↪ Logout" />
        )}
      </div>
    </div>
  )
}

function NavLink({ href, label, onClick }: { href: string; label: string; onClick?: (e: MouseEvent<HTMLAnchorElement>) => void }) {
  return (
    <a
      href={href}
      onClick={onClick}
      style={{
        display: 'block', padding: '8px 12px', color: '#888',
        textDecoration: 'none', fontSize: 13, fontFamily: 'sans-serif',
        borderRadius: 4, cursor: 'pointer',
      }}
      onMouseEnter={e => (e.currentTarget.style.background = '#1a1a1a')}
      onMouseLeave={e => (e.currentTarget.style.background = 'transparent')}
    >
      {label}
    </a>
  )
}
