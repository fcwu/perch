import { useState } from 'react'
import Sidebar from './Sidebar'
import ChatPage from './ChatPage'

interface AuthStatus {
  authenticated: boolean
  username: string
  role: string
  mode: string
  auth_method: string
}

interface ChatAppProps {
  authStatus: AuthStatus
}

export default function ChatApp({ authStatus }: ChatAppProps) {
  const [sidebarOpen, setSidebarOpen] = useState(true)
  const isAdmin = authStatus.role === 'admin' || authStatus.mode === 'single'

  // conversation_id from URL ?id=<uuid>
  const conversationId = new URLSearchParams(window.location.search).get('id') ?? undefined

  const onNewChat = () => {
    window.location.href = '/chat'
  }

  return (
    <div style={{ display: 'flex', height: '100dvh', background: '#0a0a0a', overflow: 'hidden' }}>
      {/* Sidebar */}
      {sidebarOpen && (
        <Sidebar
          isAdmin={isAdmin}
          authMethod={authStatus.auth_method}
          onNewChat={onNewChat}
          onCollapse={() => setSidebarOpen(false)}
          activeConversationId={conversationId}
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
      {/* Chat area */}
      <div style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
        <ChatPage
          userID="me"
          conversationId={conversationId}
          onSidebarToggle={() => setSidebarOpen(s => !s)}
          sidebarOpen={sidebarOpen}
        />
      </div>
    </div>
  )
}
