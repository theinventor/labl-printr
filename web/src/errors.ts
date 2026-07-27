// Forward uncaught browser errors to the labl-printr server, which relays them
// to Oopsie. The server holds the ingest key, so nothing sensitive ships in the
// bundle. Best-effort and self-silencing — error reporting must never throw.

type ClientError = { message: string; stack?: string[]; url: string; extra?: unknown };

function report(e: ClientError) {
  try {
    void fetch('/api/client-error', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(e),
      keepalive: true,
    }).catch(() => {});
  } catch {
    /* never let reporting break the app */
  }
}

function frames(stack?: string): string[] | undefined {
  if (!stack) return undefined;
  return stack
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean);
}

export function installErrorReporting() {
  window.addEventListener('error', (ev) => {
    report({
      message: ev.message || String(ev.error),
      stack: frames(ev.error?.stack),
      url: location.pathname,
      extra: { filename: ev.filename, line: ev.lineno, col: ev.colno },
    });
  });

  window.addEventListener('unhandledrejection', (ev) => {
    const reason = ev.reason;
    report({
      message: reason?.message ? `Unhandled rejection: ${reason.message}` : `Unhandled rejection: ${String(reason)}`,
      stack: frames(reason?.stack),
      url: location.pathname,
    });
  });
}
