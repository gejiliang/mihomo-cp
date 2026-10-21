import { useEffect, useMemo, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Separator } from '@/components/ui/separator';
import { Trash2Icon } from 'lucide-react';
import type { Subscription, SubscriptionInput } from '@/api/subscriptions';
import type { DiscoveredProvider } from '@/api/subscriptions';
import type { TrafficProvider } from '@/api/traffic';
import { formatApiDateTime, formatBytesExact } from '@/lib/subscription-format';
import { useI18nStore, useT } from '@/i18n';

const COMMON_TIMEZONES = [
  'UTC',
  'Asia/Shanghai',
  'Asia/Hong_Kong',
  'Asia/Tokyo',
  'Asia/Singapore',
  'Europe/London',
  'Europe/Berlin',
  'America/New_York',
  'America/Los_Angeles',
];

const GB = 1024 ** 3;

function browserTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch {
    return 'UTC';
  }
}

function validTimezone(tz: string): boolean {
  if (!tz) return true;
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: tz });
    return true;
  } catch {
    return false;
  }
}

function toDateInput(iso: string | null): string {
  if (!iso) return '';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '';
  return date.toISOString().slice(0, 10);
}

interface SubscriptionFormProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  subscription: Subscription | null;
  providers: DiscoveredProvider[];
  trafficProviders: TrafficProvider[];
  onSave: (input: SubscriptionInput, id?: string) => Promise<void>;
  onRequestDelete: (sub: Subscription) => void;
}

export function SubscriptionForm({
  open,
  onOpenChange,
  subscription,
  providers,
  trafficProviders,
  onSave,
  onRequestDelete,
}: SubscriptionFormProps) {
  const t = useT();
  const locale = useI18nStore((s) => s.locale) === 'zh' ? 'zh-CN' : 'en-GB';

  const [name, setName] = useState('');
  const [providerName, setProviderName] = useState('');
  const [providerTouched, setProviderTouched] = useState(false);
  const [url, setUrl] = useState('');
  const [enabled, setEnabled] = useState(true);
  const [quotaGB, setQuotaGB] = useState('');
  const [manualExpiry, setManualExpiry] = useState('');
  const [resetDay, setResetDay] = useState('1');
  const [resetHour, setResetHour] = useState('0');
  const [resetMinute, setResetMinute] = useState('0');
  const [timezone, setTimezone] = useState(browserTimezone());
  const [trafficProviderID, setTrafficProviderID] = useState('');
  const [formError, setFormError] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    setFormError('');
    setUrl('');
    if (subscription) {
      setName(subscription.name);
      setProviderName(subscription.provider_name || subscription.name);
      setProviderTouched(true);
      setEnabled(subscription.enabled);
      const vendorTotal = subscription.userinfo?.total ?? 0;
      setQuotaGB(
        subscription.quota_bytes !== null && vendorTotal <= 0
          ? String(Math.round((subscription.quota_bytes / GB) * 100) / 100)
          : ''
      );
      setManualExpiry(
        subscription.expires_source === 'manual' ? toDateInput(subscription.expires_at) : ''
      );
      setResetDay(String(subscription.reset_day || 1));
      setResetHour(String(subscription.reset_hour || 0));
      setResetMinute(String(subscription.reset_minute || 0));
      setTimezone(subscription.timezone || '');
      setTrafficProviderID(subscription.traffic_provider_id || '');
    } else {
      setName('');
      setProviderName('');
      setProviderTouched(false);
      setEnabled(true);
      setQuotaGB('');
      setManualExpiry('');
      setResetDay('1');
      setResetHour('0');
      setResetMinute('0');
      setTimezone(browserTimezone());
      setTrafficProviderID('');
    }
  }, [open, subscription]);

  const effectiveQuota = subscription ? formatBytesExact(subscription.quota_bytes) : null;
  const vendorExpiry =
    subscription && subscription.expires_source === 'vendor'
      ? formatApiDateTime(subscription.expires_at, locale, subscription.timezone)
      : null;

  const existingProvider = useMemo(
    () => providers.find((p) => p.name === providerName.trim()),
    [providers, providerName]
  );
  const linkedElsewhere =
    !!existingProvider?.linked_subscription_id &&
    existingProvider.linked_subscription_id !== subscription?.id;

  const handleNameChange = (value: string) => {
    setName(value);
    if (!subscription && !providerTouched) {
      setProviderName(value);
    }
  };

  const handleTimezoneChange = (value: string) => {
    setTimezone(value);
  };

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setFormError('');

    const trimmedName = name.trim();
    const trimmedProvider = providerName.trim() || trimmedName;
    const trimmedUrl = url.trim();
    const quota = quotaGB.trim() === '' ? null : Number(quotaGB);
    const day = Number(resetDay);
    const hour = Number(resetHour);
    const minute = Number(resetMinute);

    if (!trimmedName) {
      setFormError(t('subscriptions.formInvalid'));
      return;
    }
    if (
      !Number.isInteger(day) ||
      day < 1 ||
      day > 31 ||
      !Number.isInteger(hour) ||
      hour < 0 ||
      hour > 23 ||
      !Number.isInteger(minute) ||
      minute < 0 ||
      minute > 59
    ) {
      setFormError(t('subscriptions.formInvalid'));
      return;
    }
    if (quota !== null && (!Number.isFinite(quota) || quota <= 0)) {
      setFormError(t('subscriptions.formInvalid'));
      return;
    }
    if (!validTimezone(timezone.trim())) {
      setFormError(t('subscriptions.formInvalid'));
      return;
    }
    if (linkedElsewhere) {
      setFormError(t('subscriptions.providerConflict'));
      return;
    }
    if (
      subscription &&
      trimmedProvider !== subscription.provider_name &&
      !existingProvider
    ) {
      setFormError(t('subscriptions.providerMustExist'));
      return;
    }
    if (!existingProvider && !trimmedUrl) {
      setFormError(t('subscriptions.urlRequired'));
      return;
    }

    const input: SubscriptionInput = {
      name: trimmedName,
      provider_name: trimmedProvider,
      enabled,
      reset_day: day,
      reset_hour: hour,
      reset_minute: minute,
      timezone: timezone.trim(),
    };
    // Linking an existing provider must not send a URL (the backend rejects
    // it and would otherwise overwrite the shared provider entry).
    if (!existingProvider && trimmedUrl) input.url = trimmedUrl;
    // Explicit 0 means "no manual quota supplement" on update; null would
    // keep the previous manual value.
    input.quota_bytes = quota !== null ? Math.round(quota * GB) : 0;
    // Empty string clears a manual expiry; null/omitted would keep it.
    input.manual_expire = manualExpiry ? `${manualExpiry}T00:00:00Z` : '';
    input.traffic_provider_id = trafficProviderID;

    setSaving(true);
    try {
      await onSave(input, subscription?.id);
      onOpenChange(false);
    } catch {
      // The page reports the sanitized error toast; keep the form open so the
      // user does not lose input. Never echo the URL in an error message.
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(o) => onOpenChange(o)}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {subscription ? t('subscriptions.edit') : t('subscriptions.add')}
          </DialogTitle>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-4" autoComplete="off">
          <section className="space-y-3">
            <h3 className="text-sm font-medium text-muted-foreground">
              {t('subscriptions.sectionBasic')}
            </h3>
            <div className="space-y-1.5">
              <Label htmlFor="subscription-name">{t('common.name')}</Label>
              <Input
                id="subscription-name"
                value={name}
                onChange={(e) => handleNameChange(e.target.value)}
                placeholder="Example subscription"
                required
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="subscription-provider">{t('subscriptions.providerName')}</Label>
              <Input
                id="subscription-provider"
                value={providerName}
                list="subscription-provider-names"
                onChange={(e) => {
                  setProviderTouched(true);
                  setProviderName(e.target.value);
                }}
                placeholder={t('subscriptions.providerNamePlaceholder')}
              />
              <datalist id="subscription-provider-names">
                {providers.map((p) => (
                  <option key={p.name} value={p.name}>
                    {p.linked ? t('subscriptions.providerSourceLinked') : ''}
                  </option>
                ))}
              </datalist>
              <p className="text-xs text-muted-foreground">{t('subscriptions.providerHint')}</p>
              <p className="text-xs text-muted-foreground">{t('subscriptions.providerSelectHint')}</p>
              {linkedElsewhere && (
                <p className="text-xs text-destructive">{t('subscriptions.providerConflict')}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="subscription-url">{t('subscriptions.url')}</Label>
              <Input
                id="subscription-url"
                type="text"
                autoComplete="off"
                spellCheck={false}
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder={
                  subscription ? t('subscriptions.urlKeepHint') : t('subscriptions.urlPlaceholder')
                }
              />
              <p className="text-xs text-muted-foreground">
                {subscription ? t('subscriptions.urlKeepHint') : t('subscriptions.urlMaskedHint')}
              </p>
              {existingProvider && (
                <p className="text-xs text-muted-foreground">
                  {t('subscriptions.urlIgnoredForLink')}
                </p>
              )}
            </div>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                className="h-4 w-4 rounded border border-input"
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
              />
              {t('subscriptions.enabled')}
            </label>
          </section>

          <Separator />

          <section className="space-y-3">
            <h3 className="text-sm font-medium text-muted-foreground">
              {t('subscriptions.sectionQuota')}
            </h3>
            <div className="space-y-1.5">
              <Label htmlFor="subscription-quota">{t('subscriptions.quotaGB')}</Label>
              <Input
                id="subscription-quota"
                type="number"
                min={0}
                step="0.01"
                value={quotaGB}
                onChange={(e) => setQuotaGB(e.target.value)}
                placeholder={effectiveQuota ?? ''}
              />
              <p className="text-xs text-muted-foreground">
                {t('subscriptions.quotaGBHint')}
                {effectiveQuota
                  ? ` ${t('subscriptions.effectiveQuota', { value: effectiveQuota })}`
                  : ''}
              </p>
            </div>
            <div className="grid grid-cols-3 gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="subscription-reset-day">{t('subscriptions.resetDay')}</Label>
                <Input
                  id="subscription-reset-day"
                  type="number"
                  min={1}
                  max={31}
                  value={resetDay}
                  onChange={(e) => setResetDay(e.target.value)}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="subscription-reset-hour">{t('subscriptions.resetHour')}</Label>
                <Input
                  id="subscription-reset-hour"
                  type="number"
                  min={0}
                  max={23}
                  value={resetHour}
                  onChange={(e) => setResetHour(e.target.value)}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="subscription-reset-minute">{t('subscriptions.resetMinute')}</Label>
                <Input
                  id="subscription-reset-minute"
                  type="number"
                  min={0}
                  max={59}
                  value={resetMinute}
                  onChange={(e) => setResetMinute(e.target.value)}
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="subscription-timezone">{t('subscriptions.timezone')}</Label>
              <Input
                id="subscription-timezone"
                value={timezone}
                list="subscription-timezones"
                onChange={(e) => handleTimezoneChange(e.target.value)}
                placeholder={t('subscriptions.serverTimezone')}
              />
              <datalist id="subscription-timezones">
                {[...new Set([browserTimezone(), ...COMMON_TIMEZONES])].map((tz) => (
                  <option key={tz} value={tz} />
                ))}
              </datalist>
              <p className="text-xs text-muted-foreground">{t('subscriptions.resetHint')}</p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="subscription-traffic-provider">{t('subscriptions.localProvider')}</Label>
              <select
                id="subscription-traffic-provider"
                className="flex h-8 w-full rounded-lg border border-input bg-transparent px-2.5 py-1 text-sm"
                value={trafficProviderID}
                onChange={(e) => setTrafficProviderID(e.target.value)}
              >
                <option value="">{t('subscriptions.localProviderNone')}</option>
                {trafficProviders.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
              <p className="text-xs text-muted-foreground">{t('subscriptions.localProviderHint')}</p>
            </div>
          </section>

          <Separator />

          <section className="space-y-3">
            <h3 className="text-sm font-medium text-muted-foreground">
              {t('subscriptions.sectionExpiry')}
            </h3>
            <div className="space-y-1.5">
              <Label htmlFor="subscription-expiry">{t('subscriptions.manualExpiry')}</Label>
              <Input
                id="subscription-expiry"
                type="date"
                value={manualExpiry}
                onChange={(e) => setManualExpiry(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                {t('subscriptions.manualExpiryHint')} {t('subscriptions.clearManualExpiry')}
                {vendorExpiry ? ` ${t('subscriptions.vendorExpiry', { value: vendorExpiry })}` : ''}
              </p>
            </div>
          </section>

          {formError && <p className="text-sm text-destructive">{formError}</p>}

          <DialogFooter className="flex items-center justify-between sm:justify-between">
            <div>
              {subscription && (
                <Button
                  type="button"
                  variant="destructive"
                  size="sm"
                  onClick={() => onRequestDelete(subscription)}
                >
                  <Trash2Icon className="mr-1 h-4 w-4" />
                  {t('subscriptions.delete')}
                </Button>
              )}
            </div>
            <div className="flex gap-2">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" disabled={saving || linkedElsewhere}>
                {saving ? t('common.saving') : t('common.save')}
              </Button>
            </div>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
