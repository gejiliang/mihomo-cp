import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { PlusIcon, ActivityIcon, LoaderIcon } from 'lucide-react';
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
  const [countryFilter, setCountryFilter] = useState('');
  const [formOpen, setFormOpen] = useState(false);
  const [editingProxy, setEditingProxy] = useState<Proxy | undefined>(undefined);
  const [testAllTrigger, setTestAllTrigger] = useState(0);
  const [testingAll, setTestingAll] = useState(false);

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

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t('proxies.title')}</h1>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            onClick={() => { setTestingAll(true); setTestAllTrigger((n) => n + 1); }}
            disabled={testingAll}
          >
            {testingAll ? (
              <>
                <LoaderIcon className="h-3.5 w-3.5 animate-spin" />
                {t('proxies.testing')}
              </>
            ) : (
              <>
                <ActivityIcon className="h-3.5 w-3.5" />
                {t('proxies.testAllDelay')}
              </>
            )}
          </Button>
          <Button onClick={handleAddClick}>
            <PlusIcon />
            {t('proxies.add')}
          </Button>
        </div>
      </div>

      <div className="flex items-center gap-2">
        <Input
          type="text"
          placeholder={t('proxies.searchPlaceholder')}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
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
        <Select
          value={countryFilter}
          onValueChange={(v) => setCountryFilter(!v || v === '__all__' ? '' : v)}
        >
          <SelectTrigger className="w-40">
            <SelectValue placeholder={t('proxies.allCountries')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">{t('proxies.allCountries')}</SelectItem>
            {[...new Set(proxies.map((p) => p.country).filter(Boolean))].sort().map((c) => (
              <SelectItem key={c} value={c}>
                {c}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {loading ? (
        <div className="text-center py-12 text-muted-foreground">{t('common.loading')}</div>
      ) : (
        <ProxyList
          proxies={countryFilter ? proxies.filter((p) => p.country === countryFilter) : proxies}
          onEdit={handleEditClick}
          onDeleted={fetchProxies}
          onCopied={fetchProxies}
          testAllTrigger={testAllTrigger}
          onTestAllDone={() => setTestingAll(false)}
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
