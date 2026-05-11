import { useEffect, useState, useCallback } from 'react'

interface Conversation {
  id: string
  title: string
  created_at: number
  updated_at: number
  pinned: boolean
  pinned_at?: number
  runtime?: string
  model?: string
}

interface ConversationListProps {
  activeId?: string
  onSelect?: (id: string) => void
}

interface ListResponse {
  pinned?: Conversation[]
  recent?: Conversation[]
  next_before?: number
}

export default function ConversationList({ activeId, onSelect }: ConversationListProps) {
  const [pinned, setPinned] = useState<Conversation[]>([])
  const [recent, setRecent] = useState<Conversation[]>([])
  const [nextBefore, setNextBefore] = useState<number>(0)
  const [hoveredId, setHoveredId] = useState<string | null>(null)
  const [loadingMore, setLoadingMore] = useState(false)

  const loadFirstPage = useCallback(async () => {
    try {
      const r = await fetch('/api/conversations?limit=20')
      if (!r.ok) return
      const data: ListResponse = await r.json()
      setPinned(data.pinned ?? [])
      setRecent(data.recent ?? [])
      setNextBefore(data.next_before ?? 0)
    } catch { /* ignore */ }
  }, [])

  const loadMore = useCallback(async () => {
    if (!nextBefore) return
    setLoadingMore(true)
    try {
      const r = await fetch(`/api/conversations?before=${nextBefore}&limit=20`)
      if (!r.ok) return
      const data: ListResponse = await r.json()
      setRecent(prev => [...prev, ...(data.recent ?? [])])
      setNextBefore(data.next_before ?? 0)
    } catch { /* ignore */ }
    setLoadingMore(false)
  }, [nextBefore])

  useEffect(() => {
    loadFirstPage()
    const id = setInterval(loadFirstPage, 30000)
    return () => clearInterval(id)
  }, [loadFirstPage])

  useEffect(() => {
    window.addEventListener('perch:refresh-conversations', loadFirstPage)
    return () => window.removeEventListener('perch:refresh-conversations', loadFirstPage)
  }, [loadFirstPage])

  const handleDelete = async (e: React.MouseEvent, id: string) => {
    e.preventDefault()
    e.stopPropagation()
    if (!confirm('Delete this conversation? Its scheduled prompts will also be removed.')) return
    await fetch(`/api/conversations/${id}`, { method: 'DELETE' })
    setPinned(prev => prev.filter(c => c.id !== id))
    setRecent(prev => prev.filter(c => c.id !== id))
    if (activeId === id) {
      window.location.href = '/chat'
    }
  }

  const handleTogglePin = async (e: React.MouseEvent, conv: Conversation) => {
    e.preventDefault()
    e.stopPropagation()
    const willPin = !conv.pinned
    // Optimistic move between groups
    if (willPin) {
      setPinned(prev => [{ ...conv, pinned: true, pinned_at: Date.now() }, ...prev])
      setRecent(prev => prev.filter(c => c.id !== conv.id))
    } else {
      setRecent(prev => [{ ...conv, pinned: false, pinned_at: undefined }, ...prev].sort((a, b) => b.updated_at - a.updated_at))
      setPinned(prev => prev.filter(c => c.id !== conv.id))
    }
    try {
      await fetch(`/api/conversations/${conv.id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ pinned: willPin }),
      })
    } catch {
      // Roll back on failure
      loadFirstPage()
    }
  }

  const FONT = "-apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif"

  const renderRow = (conv: Conversation) => (
    <div
      key={conv.id}
      style={{ position: 'relative', padding: '0 6px' }}
      onMouseEnter={() => setHoveredId(conv.id)}
      onMouseLeave={() => setHoveredId(null)}
    >
      <a
        href={`/chat?id=${conv.id}`}
        onClick={e => { if (onSelect) { e.preventDefault(); onSelect(conv.id) } }}
        style={{
          display: 'block', padding: '7px 10px',
          fontSize: 13, fontFamily: FONT,
          color: activeId === conv.id ? '#e0e0e0' : '#aaaaaa',
          background: activeId === conv.id ? 'rgba(255,255,255,0.08)' : hoveredId === conv.id ? 'rgba(255,255,255,0.05)' : 'transparent',
          textDecoration: 'none',
          whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
          paddingRight: hoveredId === conv.id ? 56 : 10,
          borderRadius: 7,
          transition: 'background 0.1s, color 0.1s',
        }}
      >
        {conv.title}
      </a>
      {hoveredId === conv.id && (
        <>
          <button
            onClick={(e) => handleTogglePin(e, conv)}
            style={{
              position: 'absolute', right: 30, top: '50%', transform: 'translateY(-50%)',
              background: 'none', border: 'none', color: conv.pinned ? '#ffd166' : '#666', cursor: 'pointer',
              fontSize: 13, padding: '2px 4px', display: 'flex', alignItems: 'center',
            }}
            title={conv.pinned ? 'Unpin' : 'Pin'}
            aria-label={conv.pinned ? 'unpin' : 'pin'}
          >📌</button>
          <button
            onClick={(e) => handleDelete(e, conv.id)}
            style={{
              position: 'absolute', right: 8, top: '50%', transform: 'translateY(-50%)',
              background: 'none', border: 'none', color: '#555', cursor: 'pointer',
              fontSize: 13, padding: '2px 4px', display: 'flex', alignItems: 'center',
            }}
            title="Delete conversation"
          >✕</button>
        </>
      )}
    </div>
  )

  const groupHeader = (label: string) => (
    <div style={{
      padding: '10px 12px 3px', fontSize: 11, color: '#5e5e5e',
      fontFamily: FONT, letterSpacing: '0.04em', fontWeight: 600,
    }}>{label}</div>
  )

  return (
    <div>
      {pinned.length > 0 && (
        <div>
          {groupHeader('Pinned')}
          {pinned.map(renderRow)}
        </div>
      )}
      {recent.length > 0 && (
        <div>
          {groupHeader('Recent')}
          {recent.map(renderRow)}
        </div>
      )}
      {nextBefore > 0 && (
        <div style={{ padding: '8px 16px' }}>
          <button
            onClick={loadMore}
            disabled={loadingMore}
            style={{
              width: '100%', background: 'rgba(255,255,255,0.04)', border: '1px solid #333',
              color: '#aaa', borderRadius: 6, padding: '6px 12px', fontSize: 12,
              fontFamily: FONT, cursor: loadingMore ? 'default' : 'pointer',
            }}
          >{loadingMore ? 'Loading…' : 'Load more'}</button>
        </div>
      )}
      {pinned.length === 0 && recent.length === 0 && (
        <div style={{ padding: '16px 12px', color: '#5e5e5e', fontSize: 12, fontFamily: FONT }}>
          No conversations yet
        </div>
      )}
    </div>
  )
}
