import { useEffect, useRef, useState } from 'react'

type Handlers = Record<string, (data: unknown) => void>

export type SSEStatus = 'connecting' | 'live' | 'offline'

/**
 * Subscribes to the Go SSE hub for one channel (a room code, or "lobby").
 * EventSource reconnects on its own, so this only tracks status for the UI and
 * routes named events at the latest handler set — handlers are read through a
 * ref so re-renders never tear down the stream.
 */
export function useSSE(channel: string | null, handlers: Handlers): SSEStatus {
  const [status, setStatus] = useState<SSEStatus>('connecting')
  const handlersRef = useRef(handlers)
  handlersRef.current = handlers

  const eventNames = Object.keys(handlers).join(',')

  useEffect(() => {
    if (!channel) return

    const source = new EventSource(`/api/events?table_id=${encodeURIComponent(channel)}`)
    const names = eventNames ? eventNames.split(',') : []

    const listeners = names.map((name) => {
      const listener = (event: MessageEvent<string>) => {
        let payload: unknown = null
        try {
          payload = JSON.parse(event.data)
        } catch {
          payload = event.data
        }
        handlersRef.current[name]?.(payload)
      }
      source.addEventListener(name, listener)
      return [name, listener] as const
    })

    const onOpen = () => setStatus('live')
    const onError = () => setStatus(source.readyState === EventSource.CLOSED ? 'offline' : 'connecting')
    source.addEventListener('open', onOpen)
    source.addEventListener('error', onError)
    source.addEventListener('connected', onOpen)

    return () => {
      for (const [name, listener] of listeners) source.removeEventListener(name, listener)
      source.removeEventListener('open', onOpen)
      source.removeEventListener('error', onError)
      source.removeEventListener('connected', onOpen)
      source.close()
    }
  }, [channel, eventNames])

  return status
}
