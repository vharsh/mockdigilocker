import { portal } from '../lib/api';
import { Link } from '../lib/router';
import { Card, KV, Loading, Alert, Badge, useAsync, PageHeader } from '../components/ui';
import { fmtDate, genderLabel, timeAgo } from '../lib/format';
import { ActivityList } from './Consents';

export function Dashboard() {
  const { data, error, loading } = useAsync(() => portal('GET', '/dashboard'));
  if (loading && !data) return <Loading />;
  if (error) return <Alert>{error.message}</Alert>;
  const u = data.user;
  const sc = data.status_counts || {};
  const stats: [string, number, string][] = [
    ['Issued documents', data.document_count, '/documents'],
    ['Valid', sc.valid || 0, '/documents'],
    ['Expired / invalid', (sc.expired || 0) + (sc.invalid || 0) + (sc.revoked || 0), '/documents'],
    ['Apps with access', data.active_consents, '/consents'],
  ];
  return (
    <div>
      <PageHeader title={`Hello, ${u.name.split(' ')[0]}`} subtitle={<>DigiLocker ID <code className="rounded bg-slate-100 px-1">{u.id}</code>{u.persona && <> · persona <Badge tone="violet">{u.persona}</Badge></>}</>} />
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        {stats.map(([l, n, to]) => (
          <Link key={l} to={to} className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm transition hover:border-brand-500">
            <div className="text-3xl font-bold text-slate-900">{n}</div>
            <div className="text-sm text-slate-500">{l}</div>
          </Link>
        ))}
      </div>
      <div className="mt-6 grid gap-6 lg:grid-cols-3">
        <Card title="Profile" className="lg:col-span-1" actions={<Link to="/settings" className="text-xs font-semibold text-brand-600">Edit</Link>}>
          <KV className="sm:grid-cols-1" items={[
            ['Name', u.name], ['Date of birth', fmtDate(u.dob)], ['Gender', genderLabel(u.gender)], ['Mobile', u.mobile], ['E-mail', u.email],
            ['e-Aadhaar linked', u.eaadhaar ? <Badge tone="green">Yes</Badge> : <Badge>No</Badge>],
          ]} />
        </Card>
        <Card title="Recent activity" className="lg:col-span-2" actions={<Link to="/activity" className="text-xs font-semibold text-brand-600">View all</Link>}>
          <ActivityList items={data.recent_activity} />
        </Card>
      </div>
      <p className="mt-6 text-xs text-slate-500">Account created {fmtDate(u.created_at)} · last updated {timeAgo(u.updated_at)}</p>
    </div>
  );
}
