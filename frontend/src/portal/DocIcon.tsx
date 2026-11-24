import { cx } from '../components/ui';

const colors: Record<string, string> = {
  ADHAR: 'bg-orange-100 text-orange-700', PANCR: 'bg-sky-100 text-sky-700', DRVLC: 'bg-emerald-100 text-emerald-700', RVCER: 'bg-violet-100 text-violet-700',
};

export function DocIcon({ type, className }: { type: string; className?: string }) {
  return <div className={cx('grid h-11 w-11 shrink-0 place-items-center rounded-lg font-mono text-[10px] font-bold', colors[type] || 'bg-slate-100 text-slate-600', className)}>{type}</div>;
}
