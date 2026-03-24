import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { PlusIcon } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { ProxyList } from '@/components/proxies/proxy-list';
import { ProxyForm } from '@/components/proxies/proxy-form';
import type { Proxy } from '@/api/proxies';
import { proxiesApi } from '@/api/proxies';
import { useT } from '@/i18n';

const PROXY_TYPES = ['ss', 'trojan', 'vmess', 'vless', 'http', 'socks5', 'hysteria2', 'tuic'];

export default function ProxiesPage() {
  const t = useT();
  const [proxies, setProxies] = useState<Proxy[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState('');
  const [typeFilter, setTypeFilter] = useState('');
  const [formOpen, setFormOpen] = useState(false);
  const [editingProxy, setEditingProxy] = useState<Proxy | undefined>(undefined);

  const fetchProxies = useCallback(async () => {
    setLoading(true);
    try {
      const params: { search?: string; type?: string } = {};
      if (search) params.search = search;
      if (typeFilter) params.type = typeFilter;
      const res = await proxiesApi.list(params);
      setProxies(res.data ?? []);
    } catch {
      toast.error(t('proxies.loadFailed'));
    } finally {
      setLoading(false);
    }
  }, [search, typeFilter]);

  useEffect(() => {
    fetchProxies();
  }, [fetchProxies]);

  const handleAddClick = () => {
    setEditingProxy(undefined);
    setFormOpen(true);
  };

  const handleEditClick = (proxy: Proxy) => {
    setEditingProxy(proxy);
    setFormOpen(true);
  };

  const handleSave = async (data: { name: string; type: string; config: Record<string, unknown> }) => {
    try {
      if (editingProxy) {
        await proxiesApi.update(editingProxy.id, { name: data.name, type: data.type, config: data.config });
        toast.success(t('proxies.updated'));
      } else {
        await proxiesApi.create({ name: data.name, type: data.type, config: data.config });
        toast.success(t('proxies.created'));
      }
      setFormOpen(false);
      setEditingProxy(undefined);
      fetchProxies();
    } catch {
      toast.error(editingProxy ? t('proxies.updateFailed') : t('proxies.createFailed'));
    }
  };

  const handleSearchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setSearch(e.target.value);
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t('proxies.title')}</h1>
        <Button onClick={handleAddClick}>
          <PlusIcon />
          {t('proxies.add')}
        </Button>
      </div>

      <div className="flex items-center gap-2">
        <Input
          type="text"
          placeholder={t('proxies.searchPlaceholder')}
          value={search}
          onChange={handleSearchChange}
          className="max-w-xs"
        />
        <Select
          value={typeFilter}
          onValueChange={(v) => setTypeFilter(!v || v === '__all__' ? '' : v)}
        >
          <SelectTrigger className="w-40">
            <SelectValue placeholder={t('common.allTypes')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">{t('common.allTypes')}</SelectItem>
            {PROXY_TYPES.map((pt) => (
              <SelectItem key={pt} value={pt}>
                {pt}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {loading ? (
        <div className="text-center py-12 text-muted-foreground">{t('common.loading')}</div>
      ) : (
        <ProxyList
          proxies={proxies}
          onEdit={handleEditClick}
          onDeleted={fetchProxies}
          onCopied={fetchProxies}
        />
      )}

      <ProxyForm
        open={formOpen}
        onOpenChange={(o) => {
          setFormOpen(o);
          if (!o) setEditingProxy(undefined);
        }}
        proxy={editingProxy}
        onSave={handleSave}
      />
    </div>
  );
}
