import { useEffect, useRef, useState } from 'react';
import { toast } from 'sonner';
import { PencilIcon, CopyIcon, Trash2Icon, ActivityIcon, LoaderIcon } from 'lucide-react';
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
import type { Proxy } from '@/api/proxies';
import { proxiesApi } from '@/api/proxies';
import { runtimeApi } from '@/api/runtime';
import { useT } from '@/i18n';

interface ProxyListProps {
  proxies: Proxy[];
  onEdit: (proxy: Proxy) => void;
  onDeleted: () => void;
  onCopied: () => void;
  testAllTrigger?: number;
  onTestAllDone?: () => void;
}

const TYPE_VARIANT: Record<string, 'default' | 'secondary' | 'outline'> = {
  ss: 'default',
  trojan: 'secondary',
  vmess: 'default',
  vless: 'secondary',
  http: 'outline',
  socks5: 'outline',
  hysteria2: 'default',
  tuic: 'secondary',
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

export function ProxyList({ proxies, onEdit, onDeleted, onCopied, testAllTrigger, onTestAllDone }: ProxyListProps) {
  const t = useT();
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [confirmDescription, setConfirmDescription] = useState('');
  const [delays, setDelays] = useState<Record<string, number | 'testing' | 'timeout' | 'error'>>({});
  const testAllRef = useRef(0);

  const testOneDelay = async (proxy: Proxy) => {
    setDelays((prev) => ({ ...prev, [proxy.id]: 'testing' }));
    try {
      const res = await runtimeApi.proxyDelay(proxy.name);
      const delay = res?.data?.delay;
      if (typeof delay === 'number' && delay > 0) {
        setDelays((prev) => ({ ...prev, [proxy.id]: delay }));
      } else {
        setDelays((prev) => ({ ...prev, [proxy.id]: 'timeout' }));
      }
    } catch {
      setDelays((prev) => ({ ...prev, [proxy.id]: 'error' }));
    }
  };

  // Concurrent test all triggered by parent
  useEffect(() => {
    if (!testAllTrigger || testAllTrigger === testAllRef.current) return;
    testAllRef.current = testAllTrigger;
    Promise.all(proxies.map((p) => testOneDelay(p))).then(() => onTestAllDone?.());
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

  if (proxies.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        {t('proxies.noProxies')}
      </div>
    );
  }

  return (
    <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('common.name')}</TableHead>
            <TableHead>{t('proxies.country')}</TableHead>
            <TableHead>{t('common.type')}</TableHead>
            <TableHead>{t('proxies.server')}</TableHead>
            <TableHead>{t('proxies.delay')}</TableHead>
            <TableHead className="text-right">{t('common.actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {proxies.map((proxy) => {
            const server = proxy.config.server ? String(proxy.config.server) : '—';
            const port = proxy.config.port ? String(proxy.config.port) : '';
            const serverDisplay = port ? `${server}:${port}` : server;
            const variant = TYPE_VARIANT[proxy.type] ?? 'outline';
            const delay = delays[proxy.id];

            return (
              <TableRow key={proxy.id} className="cursor-pointer" onClick={() => onEdit(proxy)}>
                <TableCell className="font-medium">{proxy.name}</TableCell>
                <TableCell className="text-sm">{countryDisplay(proxy.country)}</TableCell>
                <TableCell>
                  <Badge variant={variant}>{proxy.type}</Badge>
                </TableCell>
                <TableCell className="text-muted-foreground">{serverDisplay}</TableCell>
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
                      onClick={() => testOneDelay(proxy)}
                      disabled={delay === 'testing'}
                      title={t('proxies.testDelay')}
                    >
                      <ActivityIcon className="h-3.5 w-3.5" />
                    </Button>
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
    </>
  );
}
