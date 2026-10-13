// data layer: single fetch client + repository functions.
import type { ApiKey, Flag } from '../domain/types';

const TOKEN_KEY = 'waver.jwt';

export const tokenStore = {
  get: () => localStorage.getItem(TOKEN_KEY),
  set: (t: string) => localStorage.setItem(TOKEN_KEY, t),
  clear: () => localStorage.removeItem(TOKEN_KEY),
};

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  const tok = tokenStore.get();
  if (tok) headers.Authorization = `Bearer ${tok}`;
  const res = await fetch(path, { ...init, headers: { ...headers, ...(init?.headers as object) } });
  if (res.status === 401) tokenStore.clear();
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error((body as { error?: string }).error ?? `request failed ${res.status}`);
  return body as T;
}

export const authRepo = {
  login: (password: string) => req<{ token: string }>('/api/v1/auth/login', { method: 'POST', body: JSON.stringify({ password }) }),
  changePassword: (current: string, next: string) =>
    req<{ status: string }>('/api/v1/auth/change-password', { method: 'POST', body: JSON.stringify({ current, next }) }),
};

export const flagRepo = {
  list: () => req<{ flags: Flag[] }>('/api/v1/flags'),
  create: (f: Omit<Flag, 'version' | 'updated_at'>) =>
    req<Flag>('/api/v1/flags', { method: 'POST', body: JSON.stringify(toUtcPayload(f)) }),
  update: (key: string, f: Omit<Flag, 'version' | 'updated_at'>) =>
    req<Flag>(`/api/v1/flags/${key}`, { method: 'PATCH', body: JSON.stringify(toUtcPayload(f)) }),
  remove: (key: string) => req<{ status: string }>(`/api/v1/flags/${key}`, { method: 'DELETE' }),
};

// Forms use datetime-local (local) -> convert to UTC unix seconds.
export function toUtcPayload(f: Omit<Flag, 'version' | 'updated_at'>) {
  return f;
}

export function datetimeLocalToUnix(s: string | undefined): number | null {
  if (!s) return null;
  const ms = new Date(s).getTime();
  return Number.isNaN(ms) ? null : Math.floor(ms / 1000);
}

export function unixToDatetimeLocal(n: number | null | undefined): string {
  if (n == null) return '';
  const d = new Date(n * 1000);
  const pad = (x: number) => String(x).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export const keyRepo = {
  list: () => req<{ keys: ApiKey[] }>('/api/v1/apikeys'),
  create: (name: string) => req<{ id: string; prefix: string; name: string; key: string }>('/api/v1/apikeys', { method: 'POST', body: JSON.stringify({ name }) }),
  revoke: (id: string) => req<{ status: string }>(`/api/v1/apikeys/${id}`, { method: 'DELETE' }),
};
