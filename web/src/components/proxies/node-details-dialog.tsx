import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import type { RuntimeProxyProvider } from '@/api/runtime';
import { protocolLabel, type NodeRow } from '@/lib/node-inventory';
import { useT } from '@/i18n';

interface NodeDetailsDialogProps {
  /** Provider row to show. Static rows use the edit form instead. */
  row: NodeRow | null;
  provider: RuntimeProxyProvider | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * Read-only details for one subscription node.
 *
 * Only the allow-listed runtime metadata is rendered. The field list is
 * explicit (no object spread), so a provider URL, server address or credential
 * can never appear here even if a raw upstream object were passed in.
 */
export function NodeDetailsDialog({ row, provider, open, onOpenChange }: NodeDetailsDialogProps) {
  const t = useT();

  const triState = (value: boolean | null | undefined) => {
    if (value === true) return t('proxies.yes');
    if (value === false) return t('proxies.no');
    return t('proxies.unknown');
  };

  const providerStatus =
    provider?.status === 'ok' ? t('proxies.providerStatusOK') : t('proxies.providerUnknown');
  const nodeCount =
    provider?.status === 'ok' && typeof provider.node_count === 'number'
      ? String(provider.node_count)
      : t('proxies.unknown');
  const updated = provider?.updated_at
    ? new Date(provider.updated_at).toLocaleString()
    : t('proxies.unknown');

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('proxies.nodeDetails')}</DialogTitle>
          <DialogDescription className="break-all">{row?.name ?? ''}</DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <DetailRow label={t('proxies.fieldProvider')} value={row?.providerName || t('proxies.unknown')} />
          <DetailRow label={t('proxies.fieldProtocol')} value={row?.protocol ? protocolLabel(row.protocol) : t('proxies.unknown')} />
          <DetailRow label={t('proxies.fieldAlive')} value={triState(row?.alive)} />
          <DetailRow label={t('proxies.fieldUDP')} value={triState(row?.udp)} />
          <DetailRow
            label={t('proxies.fieldDelay')}
            value={typeof row?.delay === 'number' && row.delay > 0 ? `${row.delay}ms` : t('proxies.unknown')}
          />
          <DetailRow label={t('proxies.fieldProviderStatus')} value={providerStatus} />
          <DetailRow label={t('proxies.nodeCount')} value={nodeCount} />
          <DetailRow label={t('proxies.fieldProviderUpdated')} value={updated} />
        </div>
        <p className="text-xs text-muted-foreground">{t('proxies.nodeDetailsDesc')}</p>
      </DialogContent>
    </Dialog>
  );
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start justify-between gap-3 text-sm">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium break-all text-right">{value}</span>
    </div>
  );
}
