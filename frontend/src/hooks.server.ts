import type { Handle } from '@sveltejs/kit';

const apiTarget = (process.env.API_INTERNAL_URL ?? 'http://api:8080').replace(/\/$/, '');

export const handle: Handle = async ({ event, resolve }) => {
  const { pathname, search } = event.url;
  if (pathname.startsWith('/api/') || pathname === '/healthz' || pathname === '/readyz' || pathname === '/metrics') {
    const targetUrl = `${apiTarget}${pathname}${search}`;
    const headers = new Headers(event.request.headers);
    headers.delete('host');
    const init: RequestInit & { duplex?: 'half' } = {
      method: event.request.method,
      headers
    };
    if (event.request.method !== 'GET' && event.request.method !== 'HEAD') {
      init.body = await event.request.arrayBuffer();
    }
    try {
      const upstream = await fetch(targetUrl, init);
      return new Response(upstream.body, {
        status: upstream.status,
        statusText: upstream.statusText,
        headers: upstream.headers
      });
    } catch {
      return new Response(JSON.stringify({ error: 'upstream_unavailable', message: 'API service is unavailable.' }), {
        status: 502,
        headers: { 'Content-Type': 'application/json' }
      });
    }
  }
  return resolve(event);
};
