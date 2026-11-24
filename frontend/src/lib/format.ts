export function fmtDate(v?: string | null, withTime = false): string {
  if (!v) return '—';
  const d = new Date(v.length === 10 ? v + 'T00:00:00' : v);
  if (isNaN(d.getTime())) return v;
  return withTime
    ? d.toLocaleString('en-IN', { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' })
    : d.toLocaleDateString('en-IN', { day: '2-digit', month: 'short', year: 'numeric' });
}

export function timeAgo(v?: string | null): string {
  if (!v) return '';
  const s = Math.round((Date.now() - new Date(v).getTime()) / 1000);
  if (s < 60) return `${Math.max(s, 0)}s ago`;
  if (s < 3600) return `${Math.round(s / 60)}m ago`;
  if (s < 86400) return `${Math.round(s / 3600)}h ago`;
  return `${Math.round(s / 86400)}d ago`;
}

export const genderLabel = (g: string) => ({ M: 'Male', F: 'Female', T: 'Transgender' } as Record<string, string>)[g] || g;

export function maskMobile(m: string) {
  return m && m.length > 4 ? '•'.repeat(m.length - 4) + m.slice(-4) : m;
}

export function statusTone(s: string): 'green' | 'red' | 'amber' | 'slate' | 'blue' {
  if (['valid', 'approved', 'delivered', 'ok'].includes(s)) return 'green';
  if (['expired', 'invalid', 'revoked', 'denied', 'failed', 'error'].includes(s)) return 'red';
  if (['unavailable', 'pending', 'retrying'].includes(s)) return 'amber';
  return 'slate';
}

export function httpTone(status: number): 'green' | 'red' | 'amber' | 'slate' {
  if (status === 0) return 'red';
  if (status < 300) return 'green';
  if (status < 400) return 'slate';
  if (status < 500) return 'amber';
  return 'red';
}

export const docTypeLabel: Record<string, string> = {
  ADHAR: 'Aadhaar Card', PANCR: 'PAN Verification Record', DRVLC: 'Driving License', RVCER: 'Vehicle Registration',
};
