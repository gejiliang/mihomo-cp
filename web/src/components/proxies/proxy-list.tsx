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
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [confirmDescription, setConfirmDescription] = useState('');

  const handleDeleteClick = async (proxy: Proxy) => {
    try {
      const res = await proxiesApi.refs(proxy.id);
      const refs = res.data ?? [];
      if (refs.length > 0) {
        setConfirmDescription(
          `This proxy is referenced by ${refs.length} group(s): ${refs.join(', ')}. Deleting it may break those groups. Continue?`
        );
      } else {
        setConfirmDescription(`Are you sure you want to delete "${proxy.name}"? This action cannot be undone.`);
      }
    } catch {
      setConfirmDescription(`Are you sure you want to delete "${proxy.name}"? This action cannot be undone.`);
    }
    setDeletingId(proxy.id);
    setConfirmOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!deletingId) return;
    try {
      await proxiesApi.delete(deletingId);
      toast.success('Proxy deleted');
      onDeleted();
    } catch {
      toast.error('Failed to delete proxy');
    } finally {
      setDeletingId(null);
    }
  };

  const handleCopy = async (proxy: Proxy) => {
    try {
      await proxiesApi.copy(proxy.id);
      toast.success(`Copied "${proxy.name}"`);
      onCopied();
    } catch {
      toast.error('Failed to copy proxy');
    }
  };

  if (proxies.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        No proxies found. Click "Add Proxy" to create one.
      </div>
    );
  }

  return (
    <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Type</TableHead>
            <TableHead>Server</TableHead>
            <TableHead className="text-right">Actions</TableHead>
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
                      title="Edit"
                    >
                      <PencilIcon />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => handleCopy(proxy)}
                      title="Copy"
                    >
                      <CopyIcon />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => handleDeleteClick(proxy)}
                      title="Delete"
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
        title="Delete Proxy"
        description={confirmDescription}
        onConfirm={handleDeleteConfirm}
        variant="destructive"
      />
    </>
  );
}
