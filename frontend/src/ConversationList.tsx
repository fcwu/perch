import { useEffect, useState, useCallback } from 'react'

interface Conversation {
  id: string
  title: string
  created_at: number
  updated_at: number
}

interface ConversationListProps {
  activeId?: string
}

// Group conversations by recency relative to now
function groupConversations(convs: Conversation[]) {
  const now = Date.now()
  const dayMs = 86400000
  const groups: { label: string; items: Conversation[] }[] = [
    { label: 'Today', items: [] },
    { label: 'Yesterday', items: [] },
    { label: 'Last 7 Days', items: [] },
    { label: 'Older', items: [] },
  ]
  for (const c of convs) {
    const age = now - c.updated_at * 1000
    if (age < dayMs) groups[0].items.push(c)
    else if (age < 2 * dayMs) groups[1].items.push(c)
    else if (age < 7 * dayMs) groups[2].items.push(c)
    else groups[3].items.push(c)
  }
  return groups.filter(g => g.items.length > 0)
}

export default function ConversationList({ activeId }: ConversationListProps) {
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [hoveredId, setHoveredId] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const r = await fetch('/api/conversations')
      if (r.ok) setConversations(await r.json())
    } catch { /* ignore */ }
  }, [])

  useEffect(() => {
    load()
    const id = setInterval(load, 30000)
    return () => clearInterval(id)
  }, [load])

  // Listen for refresh events (emitted after a new conversation is created)
  useEffect(() => {
    window.addEventListener('perch:refresh-conversations', load)
    return () => window.removeEventListener('perch:refresh-conversations', load)
  }, [load])

  const handleDelete = async (e: React.MouseEvent, id: string) => {
    e.preventDefault()
    e.stopPropagation()
    await fetch(`/api/conversations/${id}`, { method: 'DELETE' })
    setConversations(prev => prev.filter(c => c.id !== id))
    if (activeId === id) {
      window.location.href = '/chat'
    }
  }

  const groups = groupConversations(conversations)

  return (
    <div>
      {groups.map(group => (
        <div key={group.label}>
          <div style={{ padding: '10px 12px 3px', fontSize: 11, color: '#5e5e5e', fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif", letterSpacing: '0.04em', fontWeight: 600 }}>
            {group.label}
          </div>
          {group.items.map(conv => (
            <div
              key={conv.id}
              style={{ position: 'relative', padding: '0 6px' }}
              onMouseEnter={() => setHoveredId(conv.id)}
              onMouseLeave={() => setHoveredId(null)}
            >
              <a
                href={`/chat?id=${conv.id}`}
                style={{
                  display: 'block', padding: '7px 10px',
                  fontSize: 13, fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif",
                  color: activeId === conv.id ? '#e0e0e0' : '#aaaaaa',
                  background: activeId === conv.id ? 'rgba(255,255,255,0.08)' : hoveredId === conv.id ? 'rgba(255,255,255,0.05)' : 'transparent',
                  textDecoration: 'none',
                  whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
                  paddingRight: hoveredId === conv.id ? 32 : 10,
                  borderRadius: 7,
                  transition: 'background 0.1s, color 0.1s',
                }}
              >
                {conv.title}
              </a>
              {hoveredId === conv.id && (
                <button
                  onClick={(e) => handleDelete(e, conv.id)}
                  style={{
                    position: 'absolute', right: 12, top: '50%', transform: 'translateY(-50%)',
                    background: 'none', border: 'none', color: '#555', cursor: 'pointer',
                    fontSize: 13, padding: '2px 4px', display: 'flex', alignItems: 'center',
                  }}
                  title="Delete conversation"
                >✕</button>
              )}
            </div>
          ))}
        </div>
      ))}
      {conversations.length === 0 && (
        <div style={{ padding: '16px 12px', color: '#5e5e5e', fontSize: 12, fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif" }}>
          No conversations yet
        </div>
      )}
    </div>
  )
}
