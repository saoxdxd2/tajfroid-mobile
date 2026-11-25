// Vercel Edge Middleware for mobile.tajfroid.com stealth 404 isolation.
// Any unauthenticated browser request outside QR pairing returns a plain 404 Not Found.
export const config = {
  matcher: ['/((?!assets/|favicon\\.svg|manifest\\.json|manifest\\.webmanifest|pwa-512x512\\.png|sw\\.js|icons\\.svg).*)'],
};

export default function middleware(request: Request): Response | undefined {
  const url = new URL(request.url);
  const path = url.pathname;

  // API endpoints enforce their own token/cookie stealth 404 inside Go router
  if (path.startsWith('/api/')) {
    return undefined;
  }

  // Allow QR pairing bootstrap URL when a pairing code or relay param is supplied
  if (path === '/pair' && url.searchParams.has('code')) {
    return undefined;
  }

  // Allow already-paired phones carrying the HttpOnly device cookie
  const cookies = request.headers.get('cookie') || '';
  if (/(?:^|;\s*)taj_device_token=[^;\s]+/.test(cookies)) {
    return undefined;
  }

  // Allow explicit bearer header (native/TWA shell probes)
  const auth = request.headers.get('authorization') || '';
  if (/^Bearer\s+\S+/i.test(auth)) {
    return undefined;
  }

  // Stealth 404: hide the existence of the mobile console from public web scanners
  return new Response('404 Not Found', {
    status: 404,
    headers: {
      'content-type': 'text/plain; charset=utf-8',
      'cache-control': 'no-store',
    },
  });
}
