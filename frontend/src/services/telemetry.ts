export type ConsoleTelemetryEvent =
  | 'page_view'
  | 'create_service'
  | 'instance_action'
  | 'order_filter'
  | 'renew_order'
  | 'admin_action'

function viewport() {
  if (window.innerWidth < 640) return 'mobile'
  if (window.innerWidth < 1024) return 'tablet'
  return 'desktop'
}

// Keep payloads anonymous: no user, order, instance, node, free-text, or token fields belong here.
export function trackConsoleEvent(
  event: ConsoleTelemetryEvent,
  area: 'me' | 'super',
  page: string,
  fields: {
    action?: string
    result?: 'success' | 'error' | 'started'
    durationMs?: number
  } = {}
) {
  const payload = JSON.stringify({
    event,
    area,
    page,
    action: fields.action ?? '',
    result: fields.result ?? '',
    durationMs: Math.max(0, Math.round(fields.durationMs ?? 0)),
    viewport: viewport()
  })
  void fetch('/api/telemetry/console-events', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: payload,
    credentials: 'same-origin',
    keepalive: true
  }).catch(() => undefined)
}
