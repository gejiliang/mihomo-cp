import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { toast } from 'sonner';
import { AlertTriangleIcon, ExternalLinkIcon, PlusIcon, RefreshCwIcon, RssIcon } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { ConfirmDialog } from '@/components/shared/confirm-dialog';
import { SubscriptionCard } from '@/components/subscriptions/subscription-card';
import { SubscriptionForm } from '@/components/subscriptions/subscription-form';
import { subscriptionsApi, type DiscoveredProvider, type Subscription, type SubscriptionInput } from '@/api/subscriptions';
import { readApiError } from '@/lib/api-error';
import { trafficApi, type TrafficProvider } from '@/api/traffic';
import { useAuthStore } from '@/stores/auth';
import { useT } from '@/i18n';

export default function SubscriptionsPage() {
  const t = useT();
  const isAdmin = useAuthStore((s) => s.user?.role === 'admin');

  const [subscriptions, setSubscriptions] = useState<Subscription[]>([]);
  const [providers, setProviders] = useState<DiscoveredProvider[]>([]);
  const [trafficProviders, setTrafficProviders] = useState<TrafficProvider[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [refreshing, setRefreshing] = useState<Record<string, boolean>>({});
  const [refreshingAll, setRefreshingAll] = useState(false);
  const [draftNotice, setDraftNotice] = useState(false);

  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Subscription | null>(null);
  const [deleting, setDeleting] = useState<Subscription | null>(null);

  const fetchAll = useCallback(async () => {
    try {
      const [subs, provs, tps] = await Promise.all([
        subscriptionsApi.list(),
        subscriptionsApi.providers().catch(() => ({ data: [] as DiscoveredProvider[] })),
        trafficApi.providers().catch(() => ({ data: [] as TrafficProvider[] })),
      ]);
      setSubscriptions(subs.data ?? []);
      setProviders(provs.data ?? []);
      setTrafficProviders(tps.data ?? []);
      setLoadError(false);
    } catch {
      setLoadError(true);
      toast.error(t('subscriptions.loadFailed'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchAll();
    const timer = setInterval(fetchAll, 60_000);
    return () => clearInterval(timer);
  }, [fetchAll]);

  const openCreate = () => {
    setEditing(null);
    setFormOpen(true);
  };

  const openEdit = (sub: Subscription) => {
    setEditing(sub);
    setFormOpen(true);
  };

  const save = async (input: SubscriptionInput, id?: string) => {
    try {
      const res = id
        ? await subscriptionsApi.update(id, input)
        : await subscriptionsApi.create(input);
      if (res.data?.draft_changed) setDraftNotice(true);
      toast.success(id ? t('subscriptions.updated') : t('subscriptions.created'));
      await fetchAll();
    } catch (err) {
      const info = await readApiError(err);
      toast.error(
        info.status === 409
          ? t('subscriptions.saveConflict')
          : info.message || t('subscriptions.saveFailed')
      );
      throw new Error('save failed');
    }
  };

  const confirmDelete = async () => {
    if (!deleting) return;
    try {
      const res = await subscriptionsApi.remove(deleting.id);
      toast.success(t('subscriptions.deleted'));
      if (res.data?.draft_changed !== false) setDraftNotice(true);
      setDeleting(null);
      await fetchAll();
    } catch (err) {
      const info = await readApiError(err);
      if (info.references && info.references.length > 0) {
        toast.error(t('subscriptions.deleteBlocked', { groups: info.references.join(', ') }));
      } else if (info.status === 409) {
        toast.error(info.message || t('subscriptions.deleteReferencedGeneric'));
      } else {
        toast.error(info.message || t('subscriptions.deleteFailed'));
      }
    }
  };

  const refreshOne = async (sub: Subscription) => {
    if (!isAdmin) return;
    setRefreshing((prev) => ({ ...prev, [sub.id]: true }));
    try {
      const res = await subscriptionsApi.refresh(sub.id);
      const updated = res.data?.subscription;
      if (updated) {
        setSubscriptions((prev) => prev.map((s) => (s.id === updated.id ? updated : s)));
      }
      const err = res.data?.error;
      if (err) {
        if (err.code === 'rate_limited' && err.retry_after_seconds) {
          toast.error(
            t('subscriptions.rateLimited', { seconds: err.retry_after_seconds }) +
              ` ${err.message}`
          );
        } else {
          toast.error(err.message || t('subscriptions.refreshFailed'));
        }
      } else {
        toast.success(t('subscriptions.refreshed'));
      }
      await fetchAll();
    } catch (err) {
      const info = await readApiError(err);
      toast.error(info.message || t('subscriptions.refreshFailed'));
    } finally {
      setRefreshing((prev) => ({ ...prev, [sub.id]: false }));
    }
  };

  const refreshAll = async () => {
    if (!isAdmin) return;
    setRefreshingAll(true);
    const results = await Promise.allSettled(
      subscriptions
        .filter((s) => s.enabled)
        .map((s) => subscriptionsApi.refresh(s.id))
    );
    const failed = results.filter(
      (r) => r.status === 'rejected' || (r.status === 'fulfilled' && r.value.data?.error)
    ).length;
    const ok = results.length - failed;
    toast.success(t('subscriptions.refreshAllSummary', { ok, failed }));
    setRefreshingAll(false);
    await fetchAll();
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="flex items-center gap-2 text-2xl font-bold">
          <RssIcon className="h-6 w-6" />
          {t('subscriptions.title')}
        </h1>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            onClick={refreshAll}
            disabled={!isAdmin || refreshingAll || subscriptions.filter((s) => s.enabled).length === 0}
            title={isAdmin ? t('subscriptions.refreshAll') : t('common.adminOnly')}
          >
            <RefreshCwIcon className={`mr-1 h-4 w-4 ${refreshingAll ? 'animate-spin' : ''}`} />
            {refreshingAll ? t('subscriptions.refreshingAll') : t('subscriptions.refreshAll')}
          </Button>
          <Button onClick={openCreate} disabled={!isAdmin} title={isAdmin ? undefined : t('common.adminOnly')}>
            <PlusIcon className="h-4 w-4" />
            {t('subscriptions.add')}
          </Button>
        </div>
      </div>

      {!isAdmin && (
        <p className="text-xs text-muted-foreground">{t('subscriptions.readonlyHint')}</p>
      )}

      {draftNotice && (
        <Alert>
          <AlertTriangleIcon />
          <AlertTitle>{t('subscriptions.draftPending')}</AlertTitle>
          <AlertDescription>
            <span>{t('subscriptions.draftPendingHint')}</span>{' '}
            <Link to="/publish" className="inline-flex items-center gap-1 font-medium text-primary hover:underline">
              {t('subscriptions.goToPublish')}
              <ExternalLinkIcon className="h-3 w-3" />
            </Link>
          </AlertDescription>
        </Alert>
      )}

      {loading ? (
        <p className="text-muted-foreground">{t('common.loading')}</p>
      ) : loadError ? (
        <Card>
          <CardContent className="space-y-3 py-8 text-center">
            <p className="text-muted-foreground">{t('subscriptions.loadFailed')}</p>
            <Button variant="outline" onClick={fetchAll}>
              {t('common.refresh')}
            </Button>
          </CardContent>
        </Card>
      ) : subscriptions.length === 0 ? (
        <Card>
          <CardContent className="py-10 text-center text-muted-foreground">
            {t('subscriptions.empty')}
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 xl:grid-cols-2">
          {subscriptions.map((sub) => (
            <SubscriptionCard
              key={sub.id}
              subscription={sub}
              refreshing={!!refreshing[sub.id]}
              isAdmin={isAdmin}
              onRefresh={refreshOne}
              onEdit={openEdit}
            />
          ))}
        </div>
      )}

      <SubscriptionForm
        open={formOpen}
        onOpenChange={(open) => {
          setFormOpen(open);
          if (!open) setEditing(null);
        }}
        subscription={editing}
        providers={providers}
        trafficProviders={trafficProviders}
        onSave={save}
        onRequestDelete={(sub) => {
          setFormOpen(false);
          setEditing(null);
          setDeleting(sub);
        }}
      />

      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        title={t('subscriptions.deleteTitle')}
        description={
          deleting ? t('subscriptions.deleteConfirm', { name: deleting.name }) : ''
        }
        onConfirm={confirmDelete}
        variant="destructive"
      />
    </div>
  );
}
