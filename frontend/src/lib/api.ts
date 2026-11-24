// Small fetch wrapper shared by the portal and the developer console.
declare global {
  interface Window { __MOCKDL__?: { apiBase?: string; publicBaseUrl?: string; env?: string; version?: string } }
}

export const runtime = {
  apiBase: (window.__MOCKDL__?.apiBase || '').replace(/\/$/, ''),
  publicBaseUrl: window.__MOCKDL__?.publicBaseUrl || location.origin,
  env: window.__MOCKDL__?.env || 'local',
  version: window.__MOCKDL__?.version || '',
};

export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string) { super(message); }
}

type Opts = { token?: string | null; body?: unknown; raw?: boolean };

export async function request<T = any>(method: string, url: string, opts: Opts = {}): Promise<T> {
  const headers: Record<string, string> = {};
  if (opts.body !== undefined) headers['Content-Type'] = 'application/json';
  if (opts.token) headers['Authorization'] = 'Bearer ' + opts.token;
  const res = await fetch(runtime.apiBase + url, {
    method, headers, credentials: 'include',
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
  });
  if (opts.raw) return res as unknown as T;
  const text = await res.text();
  let data: any = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { error: 'invalid_json', error_description: text.slice(0, 200) }; }
  if (!res.ok) {
    const err = new ApiError(res.status, data?.error || 'error', data?.error_description || data?.message || res.statusText);
    if (res.status === 401) window.dispatchEvent(new CustomEvent('mdl:unauthorized', { detail: url }));
    throw err;
  }
  return data as T;
}

// ---- token storage ----
const PORTAL = 'mdl_portal_session';
const ADMIN = 'mdl_admin_session';

export type PortalSession = { access_token: string; refresh_token: string; user: any };
export type AdminSession = { token: string; username: string; role: 'admin' | 'qa' | 'viewer'; default_password?: boolean };

function load<T>(k: string): T | null {
  try { return JSON.parse(sessionStorage.getItem(k) || 'null'); } catch { return null; }
}
export const portalStore = {
  get: () => load<PortalSession>(PORTAL),
  set: (s: PortalSession | null) => (s ? sessionStorage.setItem(PORTAL, JSON.stringify(s)) : sessionStorage.removeItem(PORTAL)),
};
export const adminStore = {
  get: () => load<AdminSession>(ADMIN),
  set: (s: AdminSession | null) => (s ? sessionStorage.setItem(ADMIN, JSON.stringify(s)) : sessionStorage.removeItem(ADMIN)),
};

export const portal = <T = any>(method: string, url: string, body?: unknown) =>
  request<T>(method, '/portal-api' + url, { token: portalStore.get()?.access_token, body });
export const admin = <T = any>(method: string, url: string, body?: unknown) =>
  request<T>(method, '/admin-api' + url, { token: adminStore.get()?.token, body });

/** Fetch a binary resource with auth and open/download it. */
export async function openBlob(url: string, token: string | undefined, download?: string) {
  const res = await fetch(runtime.apiBase + url, { headers: token ? { Authorization: 'Bearer ' + token } : {} });
  if (!res.ok) throw new ApiError(res.status, 'download_failed', 'Download failed (' + res.status + ')');
  const blob = await res.blob();
  const href = URL.createObjectURL(blob);
  if (download) {
    const a = document.createElement('a');
    a.href = href; a.download = download; a.click();
  } else window.open(href, '_blank');
  setTimeout(() => URL.revokeObjectURL(href), 60_000);
}
