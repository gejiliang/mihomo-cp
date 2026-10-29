import { useEffect, useRef, useState } from 'react';
import { toast } from 'sonner';
import {
  PencilIcon,
  CopyIcon,
  Trash2Icon,
  ActivityIcon,
  LoaderIcon,
  InfoIcon,
} from 'lucide-react';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { ConfirmDialog } from '@/components/shared/confirm-dialog';
import { NodeDetailsDialog } from '@/components/proxies/node-details-dialog';
import type { Proxy } from '@/api/proxies';
import { proxiesApi } from '@/api/proxies';
import type { RuntimeProxyProvider } from '@/api/runtime';
import { runtimeApi } from '@/api/runtime';
import { protocolLabel, type NodeRow } from '@/lib/node-inventory';
import { useT } from '@/i18n';

interface ProxyListProps {
  /** Unified static + subscription node rows, already filtered by the page. */
  rows: NodeRow[];
  providers: RuntimeProxyProvider[];
  onEdit: (proxy: Proxy) => void;
  onDeleted: () => void;
  onCopied: () => void;
  testAllTrigger?: number;
  onTestAllDone?: () => void;
  emptyMessage?: string;
}

type DelayState = number | 'testing' | 'timeout' | 'error';

const TYPE_VARIANT: Record<string, 'default' | 'secondary' | 'outline'> = {
  ss: 'default',
  trojan: 'secondary',
  vmess: 'default',
  vless: 'secondary',
  http: 'outline',
  socks5: 'outline',
  hysteria2: 'default',
  tuic: 'secondary',
  anytls: 'default',
};

const COUNTRY_FLAG: Record<string, string> = {
  HK: '\u{1F1ED}\u{1F1F0}', TW: '\u{1F1F9}\u{1F1FC}', JP: '\u{1F1EF}\u{1F1F5}',
  KR: '\u{1F1F0}\u{1F1F7}', SG: '\u{1F1F8}\u{1F1EC}', US: '\u{1F1FA}\u{1F1F8}',
  GB: '\u{1F1EC}\u{1F1E7}', DE: '\u{1F1E9}\u{1F1EA}', FR: '\u{1F1EB}\u{1F1F7}',
  CA: '\u{1F1E8}\u{1F1E6}', AU: '\u{1F1E6}\u{1F1FA}', IN: '\u{1F1EE}\u{1F1F3}',
  RU: '\u{1F1F7}\u{1F1FA}', TR: '\u{1F1F9}\u{1F1F7}', AR: '\u{1F1E6}\u{1F1F7}',
  VN: '\u{1F1FB}\u{1F1F3}', TH: '\u{1F1F9}\u{1F1ED}', ID: '\u{1F1EE}\u{1F1E9}',
  MY: '\u{1F1F2}\u{1F1FE}', BR: '\u{1F1E7}\u{1F1F7}', NL: '\u{1F1F3}\u{1F1F1}',
  IT: '\u{1F1EE}\u{1F1F9}', ES: '\u{1F1EA}\u{1F1F8}', SE: '\u{1F1F8}\u{1F1EA}',
  NO: '\u{1F1F3}\u{1F1F4}', FI: '\u{1F1EB}\u{1F1EE}', DK: '\u{1F1E9}\u{1F1F0}',
  PL: '\u{1F1F5}\u{1F1F1}', CH: '\u{1F1E8}\u{1F1ED}', IE: '\u{1F1EE}\u{1F1EA}',
  PT: '\u{1F1F5}\u{1F1F9}', NZ: '\u{1F1F3}\u{1F1FF}', ZA: '\u{1F1FF}\u{1F1E6}',
  AE: '\u{1F1E6}\u{1F1EA}', SA: '\u{1F1F8}\u{1F1E6}', IL: '\u{1F1EE}\u{1F1F1}',
  UA: '\u{1F1FA}\u{1F1E6}', PH: '\u{1F1F5}\u{1F1ED}', MX: '\u{1F1F2}\u{1F1FD}',
  MO: '\u{1F1F2}\u{1F1F4}', EG: '\u{1F1EA}\u{1F1EC}',
};

function countryDisplay(code: string): string {
  if (!code) return '';
  const flag = COUNTRY_FLAG[code];
  return flag ? `${flag} ${code}` : code;
}

export function ProxyList({
  rows,
  providers,
  onEdit,
  onDeleted,
  onCopied,
  testAllTrigger,
  onTestAllDone,
  emptyMessage,
}: ProxyListProps) {
  const t = useT();
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [confirmDescription, setConfirmDescription] = useState('');
  const [delays, setDelays] = useState<Record<string, DelayState>>({});
  const [detailsRow, setDetailsRow] = useState<NodeRow | null>(null);
  const testAllRef = useRef(0);

  const providerFor = (row: NodeRow): RuntimeProxyProvider | null =>
    row.kind === 'provider'
      ? providers.find((provider) => provider.name === row.providerName) ?? null
      : null;

  const testOneDelay = async (row: NodeRow) => {
    setDelays((prev) => ({ ...prev, [row.key]: 'testing' }));
    try {
      // Static proxies use the global runtime delay endpoint; subscription nodes
      // must use mihomo's provider-scoped healthcheck so the same node name under
      // two providers is addressed unambiguously. Both use the real node name.
      const res =
        row.kind === 'provider'
          ? await runtimeApi.providerNodeHealthcheck(row.providerName, row.name)
          : await runtimeApi.proxyDelay(row.name);
      const delay = res?.data?.delay;
      if (typeof delay === 'number' && delay > 0) {
        setDelays((prev) => ({ ...prev, [row.key]: delay }));
      } else {
        setDelays((prev) => ({ ...prev, [row.key]: 'timeout' }));
      }
    } catch {
      setDelays((prev) => ({ ...prev, [row.key]: 'error' }));
    }
  };

  // Concurrent test all triggered by parent
  useEffect(() => {
    if (!testAllTrigger || testAllTrigger === testAllRef.current) return;
    testAllRef.current = testAllTrigger;
    Promise.all(rows.map((row) => testOneDelay(row))).then(() => onTestAllDone?.());
  }, [testAllTrigger]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleDeleteClick = async (proxy: Proxy) => {
    try {
      const res = await proxiesApi.refs(proxy.id);
      const refs = res.data ?? [];
      if (refs.length > 0) {
        setConfirmDescription(
          t('proxies.deleteWithRefs', { count: refs.length, names: refs.join(', ') })
        );
      } else {
        setConfirmDescription(t('proxies.deleteConfirm', { name: proxy.name }));
      }
    } catch {
      setConfirmDescription(t('proxies.deleteConfirm', { name: proxy.name }));
    }
    setDeletingId(proxy.id);
    setConfirmOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!deletingId) return;
    try {
      await proxiesApi.delete(deletingId);
      toast.success(t('proxies.deleted'));
      onDeleted();
    } catch {
      toast.error(t('proxies.deleteFailed'));
    } finally {
      setDeletingId(null);
    }
  };

  const handleCopy = async (proxy: Proxy) => {
    try {
      await proxiesApi.copy(proxy.id);
      toast.success(t('proxies.copied'));
      onCopied();
    } catch {
      toast.error(t('proxies.copyFailed'));
    }
  };

  if (rows.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        {emptyMessage ?? t('proxies.noProxies')}
      </div>
    );
  }

  return (
    <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('common.name')}</TableHead>
            <TableHead>{t('proxies.source')}</TableHead>
            <TableHead>{t('proxies.country')}</TableHead>
            <TableHead>{t('common.type')}</TableHead>
            <TableHead>{t('proxies.server')}</TableHead>
            <TableHead>{t('proxies.delay')}</TableHead>
            <TableHead className="text-right">{t('common.actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row) => {
            const isStatic = row.kind === 'static';
            const proxy = row.proxy;
            const variant = TYPE_VARIANT[row.protocol] ?? 'outline';
            const delay = delays[row.key] ?? row.delay ?? null;

            return (
              <TableRow
                key={row.key}
                className={isStatic ? 'cursor-pointer' : undefined}
                onClick={isStatic && proxy ? () => onEdit(proxy) : undefined}
              >
                <TableCell className="font-medium">{row.name}</TableCell>
                <TableCell className="text-sm">
                  {isStatic ? (
                    <Badge variant="outline">{t('proxies.sourceStatic')}</Badge>
                  ) : (
                    <Badge variant="secondary" title={t('proxies.readOnly')} className="max-w-[180px]">
                      <span className="truncate">{row.providerName}</span>
                    </Badge>
                  )}
                </TableCell>
                <TableCell className="text-sm">{isStatic ? countryDisplay(row.country) : '—'}</TableCell>
                <TableCell>
                  <Badge variant={variant}>{protocolLabel(row.protocol)}</Badge>
                </TableCell>
                <TableCell className="text-muted-foreground">{row.server}</TableCell>
                <TableCell className="text-sm" onClick={(e) => e.stopPropagation()}>
                  {delay === 'testing' ? (
                    <LoaderIcon className="h-3.5 w-3.5 animate-spin text-muted-foreground" />
                  ) : delay === 'timeout' ? (
                    <span className="text-destructive">{t('proxies.timeout')}</span>
                  ) : delay === 'error' ? (
                    <span className="text-destructive">{t('proxies.error')}</span>
                  ) : typeof delay === 'number' ? (
                    <span className={delay < 300 ? 'text-green-600' : delay < 600 ? 'text-yellow-600' : 'text-red-600'}>
                      {delay}ms
                    </span>
                  ) : (
                    <span className="text-muted-foreground">—</span>
                  )}
                </TableCell>
                <TableCell className="text-right">
                  <div className="flex items-center justify-end gap-1" onClick={(e) => e.stopPropagation()}>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => testOneDelay(row)}
                      disabled={delay === 'testing'}
                      title={t('proxies.testDelay')}
                    >
                      <ActivityIcon className="h-3.5 w-3.5" />
                    </Button>
                    {isStatic && proxy ? (
                      <>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => onEdit(proxy)}
                          title={t('common.edit')}
                        >
                          <PencilIcon />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => handleCopy(proxy)}
                          title={t('common.copy')}
                        >
                          <CopyIcon />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => handleDeleteClick(proxy)}
                          title={t('common.delete')}
                        >
                          <Trash2Icon />
                        </Button>
                      </>
                    ) : (
                      // Subscription nodes are read-only: only delay testing and
                      // safe metadata details; no edit/copy/delete/reorder.
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => setDetailsRow(row)}
                        title={t('proxies.details')}
                      >
                        <InfoIcon />
                      </Button>
                    )}
                  </div>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('proxies.deleteTitle')}
        description={confirmDescription}
        onConfirm={handleDeleteConfirm}
        variant="destructive"
      />

      <NodeDetailsDialog
        row={detailsRow}
        provider={detailsRow ? providerFor(detailsRow) : null}
        open={detailsRow !== null}
        onOpenChange={(open) => {
          if (!open) setDetailsRow(null);
        }}
      />
    </>
  );
}
