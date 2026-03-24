import { useState } from 'react';
import { toast } from 'sonner';
import { PencilIcon, CopyIcon, Trash2Icon } from 'lucide-react';
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
import { useT } from '@/i18n';

interface ProxyListProps {
  proxies: Proxy[];
  onEdit: (proxy: Proxy) => void;
  onDeleted: () => void;
  onCopied: () => void;
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

export function ProxyList({ proxies, onEdit, onDeleted, onCopied }: ProxyListProps) {
  const t = useT();
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [confirmDescription, setConfirmDescription] = useState('');

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
            <TableHead>{t('common.type')}</TableHead>
            <TableHead>{t('proxies.server')}</TableHead>
            <TableHead className="text-right">{t('common.actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {proxies.map((proxy) => {
            const server = proxy.config.server ? String(proxy.config.server) : '—';
            const port = proxy.config.port ? String(proxy.config.port) : '';
            const serverDisplay = port ? `${server}:${port}` : server;
            const variant = TYPE_VARIANT[proxy.type] ?? 'outline';

            return (
              <TableRow key={proxy.id}>
                <TableCell className="font-medium">{proxy.name}</TableCell>
                <TableCell>
                  <Badge variant={variant}>{proxy.type}</Badge>
                </TableCell>
                <TableCell className="text-muted-foreground">{serverDisplay}</TableCell>
                <TableCell className="text-right">
                  <div className="flex items-center justify-end gap-1">
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
