import { useState, useEffect } from 'react'
import Sidebar from './Sidebar'
import ChatPage from './ChatPage'
import SettingsPanel from './SettingsPanel'

interface AuthStatus {
  authenticated: boolean
  username: string
  role: string
  mode: string
  auth_method: string
}

interface ChatAppProps {
  authStatus: AuthStatus
  accessDenied?: boolean
}

export default function ChatApp({ authStatus, accessDenied }: ChatAppProps) {
  const [sidebarOpen, setSidebarOpen] = useState(true)
  const isAdmin = authStatus.role === 'admin' || authStatus.mode === 'single'

  const [conversationId, setConversationId] = useState<string | undefined>(
    new URLSearchParams(window.location.search).get('id') ?? undefined
  )

  useEffect(() => {
    const onPopState = () => {
      setConversationId(new URLSearchParams(window.location.search).get('id') ?? undefined)
    }
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])

  const onNewChat = () => {
    window.history.pushState({}, '', '/chat')
    setConversationId(undefined)
  }

  const onSelectConversation = (id: string) => {
    window.history.pushState({}, '', `/chat?id=${id}`)
    setConversationId(id)
  }

  return (
    <>
    <div style={{ display: 'flex', height: '100dvh', background: '#0a0a0a', overflow: 'hidden' }}>
      {/* Sidebar */}
      {sidebarOpen && (
        <Sidebar
          isAdmin={isAdmin}
          authMethod={authStatus.auth_method}
          authenticated={authStatus.authenticated}
          accessDenied={accessDenied}
          onNewChat={onNewChat}
          onCollapse={() => setSidebarOpen(false)}
          activeConversationId={conversationId}
          onSelectConversation={onSelectConversation}
        />
      )}
      {/* Hamburger for mobile / collapse */}
      {!sidebarOpen && (
        <button
          onClick={() => setSidebarOpen(true)}
          style={{
            position: 'fixed', top: 12, left: 12, zIndex: 100,
            background: '#1a1a1a', border: '1px solid #333', borderRadius: 6,
            color: '#888', width: 32, height: 32, cursor: 'pointer', fontSize: 16,
          }}
        >☰</button>
      )}
      {/* Chat area — key forces remount when conversation changes */}
      <div style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
        <ChatPage
          key={conversationId ?? '__new__'}
          userID="me"
          conversationId={conversationId}
          onSidebarToggle={() => setSidebarOpen(s => !s)}
          sidebarOpen={sidebarOpen}
        />
      </div>
    </div>
    <SettingsPanel />
    </>
  )
}
