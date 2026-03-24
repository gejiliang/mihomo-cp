import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { PlusIcon, PencilIcon, Trash2Icon, RefreshCwIcon } from 'lucide-react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { ConfirmDialog } from '@/components/shared/confirm-dialog';
import type { RuleProvider } from '@/api/rule-providers';
import { ruleProvidersApi } from '@/api/rule-providers';

interface FormState {
  name: string;
  type: string;
  behavior: string;
  url: string;
  interval: string;
  path: string;
}

const DEFAULT_FORM: FormState = {
  name: '',
  type: 'http',
  behavior: 'domain',
  url: '',
  interval: '86400',
  path: '',
};

function providerToForm(p: RuleProvider): FormState {
  return {
    name: p.name,
    type: p.type,
    behavior: p.behavior,
    url: String(p.config.url ?? ''),
    interval: String(p.config.interval ?? '86400'),
    path: String(p.config.path ?? ''),
  };
}

interface ProviderFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  provider?: RuleProvider;
  onSave: (data: Partial<RuleProvider>) => void;
}

function ProviderFormDialog({ open, onOpenChange, provider, onSave }: ProviderFormDialogProps) {
  const t = useT();
  const [form, setForm] = useState<FormState>(DEFAULT_FORM);

  useEffect(() => {
    if (open) {
      setForm(provider ? providerToForm(provider) : DEFAULT_FORM);
    }
  }, [open, provider]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const config: Record<string, any> = { path: form.path };
    if (form.type === 'http') {
      config.url = form.url;
      config.interval = form.interval ? Number(form.interval) : 86400;
    }
    onSave({ name: form.name, type: form.type, behavior: form.behavior, config });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{provider ? t('ruleProviders.editDialog') : t('ruleProviders.addDialog')}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="rp-name">{t('common.name')}</Label>
            <Input
              id="rp-name"
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              placeholder="Provider name"
              required
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="rp-type">{t('common.type')}</Label>
            <Select
              value={form.type}
              onValueChange={(v) => v && setForm((p) => ({ ...p, type: v }))}
            >
              <SelectTrigger className="w-full" id="rp-type">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="http">http</SelectItem>
                <SelectItem value="file">file</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="rp-behavior">{t('ruleProviders.behavior')}</Label>
            <Select
              value={form.behavior}
              onValueChange={(v) => v && setForm((p) => ({ ...p, behavior: v }))}
            >
              <SelectTrigger className="w-full" id="rp-behavior">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="domain">domain</SelectItem>
                <SelectItem value="ipcidr">ipcidr</SelectItem>
                <SelectItem value="classical">classical</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {form.type === 'http' && (
            <>
              <div className="space-y-1.5">
                <Label htmlFor="rp-url">{t('ruleProviders.url')}</Label>
                <Input
                  id="rp-url"
                  type="url"
                  value={form.url}
                  onChange={(e) => setForm((p) => ({ ...p, url: e.target.value }))}
                  placeholder={t('ruleProviders.urlPlaceholder')}
                  required
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="rp-interval">{t('ruleProviders.intervalLabel')}</Label>
                <Input
                  id="rp-interval"
                  type="number"
                  value={form.interval}
                  onChange={(e) => setForm((p) => ({ ...p, interval: e.target.value }))}
                  placeholder={t('ruleProviders.intervalPlaceholder')}
                />
              </div>
            </>
          )}

          <div className="space-y-1.5">
            <Label htmlFor="rp-path">{t('ruleProviders.path')}</Label>
            <Input
              id="rp-path"
              value={form.path}
              onChange={(e) => setForm((p) => ({ ...p, path: e.target.value }))}
              placeholder={t('ruleProviders.pathPlaceholder')}
              required={form.type === 'file'}
            />
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit">{t('common.save')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export default function RuleProvidersPage() {
  const t = useT();
  const [providers, setProviders] = useState<RuleProvider[]>([]);
  const [loading, setLoading] = useState(false);
  const [formOpen, setFormOpen] = useState(false);
  const [editingProvider, setEditingProvider] = useState<RuleProvider | undefined>(undefined);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [refreshingId, setRefreshingId] = useState<string | null>(null);

  const fetchProviders = useCallback(async () => {
    setLoading(true);
    try {
      const res = await ruleProvidersApi.list();
      setProviders(res.data ?? []);
    } catch {
      toast.error(t('ruleProviders.loadFailed'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchProviders();
  }, [fetchProviders]);

  const handleSave = async (data: Partial<RuleProvider>) => {
    try {
      if (editingProvider) {
        await ruleProvidersApi.update(editingProvider.id, data);
        toast.success(t('ruleProviders.updated'));
      } else {
        await ruleProvidersApi.create(data);
        toast.success(t('ruleProviders.created'));
      }
      setFormOpen(false);
      setEditingProvider(undefined);
      fetchProviders();
    } catch {
      toast.error(editingProvider ? t('ruleProviders.updateFailed') : t('ruleProviders.createFailed'));
    }
  };

  const handleDeleteClick = (provider: RuleProvider) => {
    setDeletingId(provider.id);
    setConfirmOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!deletingId) return;
    try {
      await ruleProvidersApi.delete(deletingId);
      toast.success(t('ruleProviders.deleted'));
      fetchProviders();
    } catch {
      toast.error(t('ruleProviders.deleteFailed'));
    } finally {
      setDeletingId(null);
    }
  };

  const handleRefresh = async (provider: RuleProvider) => {
    setRefreshingId(provider.id);
    try {
      await ruleProvidersApi.refresh(provider.id);
      toast.success(t('ruleProviders.refreshed', { name: provider.name }));
    } catch {
      toast.error(t('ruleProviders.refreshFailed', { name: provider.name }));
    } finally {
      setRefreshingId(null);
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t('ruleProviders.title')}</h1>
        <Button onClick={() => { setEditingProvider(undefined); setFormOpen(true); }}>
          <PlusIcon />
          {t('ruleProviders.add')}
        </Button>
      </div>

      {loading ? (
        <div className="text-center py-12 text-muted-foreground">{t('common.loading')}</div>
      ) : providers.length === 0 ? (
        <div className="text-center py-12 text-muted-foreground">
          {t('ruleProviders.noProviders')}
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('common.name')}</TableHead>
              <TableHead>{t('common.type')}</TableHead>
              <TableHead>{t('ruleProviders.behavior')}</TableHead>
              <TableHead className="text-right">{t('common.actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {providers.map((provider) => (
              <TableRow key={provider.id}>
                <TableCell className="font-medium">{provider.name}</TableCell>
                <TableCell>
                  <Badge variant="secondary">{provider.type}</Badge>
                </TableCell>
                <TableCell>
                  <Badge variant="outline">{provider.behavior}</Badge>
                </TableCell>
                <TableCell className="text-right">
                  <div className="flex items-center justify-end gap-1">
                    {provider.type === 'http' && (
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => handleRefresh(provider)}
                        disabled={refreshingId === provider.id}
                        title={t('common.refresh')}
                      >
                        <RefreshCwIcon className={`h-4 w-4 ${refreshingId === provider.id ? 'animate-spin' : ''}`} />
                      </Button>
                    )}
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => { setEditingProvider(provider); setFormOpen(true); }}
                      title={t('common.edit')}
                    >
                      <PencilIcon className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => handleDeleteClick(provider)}
                      title={t('common.delete')}
                    >
                      <Trash2Icon className="h-4 w-4" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <ProviderFormDialog
        open={formOpen}
        onOpenChange={(o) => {
          setFormOpen(o);
          if (!o) setEditingProvider(undefined);
        }}
        provider={editingProvider}
        onSave={handleSave}
      />

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('ruleProviders.deleteTitle')}
        description={t('ruleProviders.deleteConfirm')}
        onConfirm={handleDeleteConfirm}
        variant="destructive"
      />
    </div>
  );
}
