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

function zoneOffsetMinutes(date: Date, timeZone: string): number {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: timeZone || undefined,
    hour12: false,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).formatToParts(date);
  const get = (type: string) => Number(parts.find((p) => p.type === type)?.value ?? 0);
  const asUTC = Date.UTC(
    get('year'),
    get('month') - 1,
    get('day'),
    get('hour') % 24,
    get('minute'),
    get('second')
  );
  return Math.round((asUTC - date.getTime()) / 60000);
}

/** Formats an instant as a wall-clock datetime-local value in the chosen zone. */
function toDateTimeInput(iso: string | null, timeZone: string): string {
  if (!iso) return '';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '';
  try {
    const parts = new Intl.DateTimeFormat('en-CA', {
      timeZone: timeZone || undefined,
      hour12: false,
      hourCycle: 'h23',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    }).formatToParts(date);
    const get = (type: string) => parts.find((p) => p.type === type)?.value ?? '00';
    return `${get('year')}-${get('month')}-${get('day')}T${get('hour')}:${get('minute')}:${get('second')}`;
  } catch {
    return '';
  }
}

/** Converts an explicit wall-clock datetime in an IANA zone to RFC3339 UTC. */
function wallTimeToISO(value: string, timeZone: string): string | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/.exec(value);
  if (!match) return null;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const hour = Number(match[4]);
  const minute = Number(match[5]);
  const second = Number(match[6] ?? '0');
  if (!year || month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 || second > 59) {
    return null;
  }
  const wallUTC = Date.UTC(year, month - 1, day, hour, minute, second);
  try {
    if (timeZone) new Intl.DateTimeFormat('en-US', { timeZone });
    let instant = wallUTC - zoneOffsetMinutes(new Date(wallUTC), timeZone) * 60000;
    instant = wallUTC - zoneOffsetMinutes(new Date(instant), timeZone) * 60000;
    return new Date(instant).toISOString();
  } catch {
    return null;
  }
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
  const [quotaDirty, setQuotaDirty] = useState(false);
  const [manualExpiry, setManualExpiry] = useState('');
  const [expiryDirty, setExpiryDirty] = useState(false);
  const [expiryTimezone, setExpiryTimezone] = useState(browserTimezone());
  const [resetConfigured, setResetConfigured] = useState(false);
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
      const expiryZone = subscription.timezone || browserTimezone();
      setExpiryTimezone(expiryZone);
      setQuotaDirty(false);
      setExpiryDirty(false);
      setManualExpiry(
        subscription.expires_source === 'manual'
          ? toDateTimeInput(subscription.expires_at, expiryZone)
          : ''
      );
      const hasReset = subscription.reset_day >= 1;
      setResetConfigured(hasReset);
      setResetDay(hasReset ? String(subscription.reset_day) : '1');
      setResetHour(String(subscription.reset_hour || 0));
      setResetMinute(String(subscription.reset_minute || 0));
      // Empty timezone means server-local for an existing valid schedule;
      // never silently replace it with the browser zone.
      setTimezone(subscription.timezone || '');
      setTrafficProviderID(subscription.traffic_provider_id || '');
    } else {
      setName('');
      setProviderName('');
      setProviderTouched(false);
      setEnabled(true);
      setQuotaGB('');
      setQuotaDirty(false);
      setManualExpiry('');
      setExpiryDirty(false);
      setExpiryTimezone(browserTimezone());
      setResetConfigured(false);
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
    const manualExpiryISO = manualExpiry ? wallTimeToISO(manualExpiry, expiryTimezone) : '';
    const day = resetConfigured ? Number(resetDay) : 0;
    const hour = resetConfigured ? Number(resetHour) : 0;
    const minute = resetConfigured ? Number(resetMinute) : 0;

    if (!trimmedName) {
      setFormError(t('subscriptions.formInvalid'));
      return;
    }
    if (
      resetConfigured &&
      (!Number.isInteger(day) ||
        day < 1 ||
        day > 31 ||
        !Number.isInteger(hour) ||
        hour < 0 ||
        hour > 23 ||
        !Number.isInteger(minute) ||
        minute < 0 ||
        minute > 59)
    ) {
      setFormError(t('subscriptions.formInvalid'));
      return;
    }
    if (quota !== null && (!Number.isFinite(quota) || quota <= 0)) {
      setFormError(t('subscriptions.formInvalid'));
      return;
    }
    if (resetConfigured && !validTimezone(timezone.trim())) {
      setFormError(t('subscriptions.formInvalid'));
      return;
    }
    if (expiryDirty && manualExpiry && manualExpiryISO === null) {
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
      timezone: resetConfigured ? timezone.trim() : '',
    };
    // Creating against an existing provider must omit the URL (link-only).
    // Editing the current subscription's own provider may update its URL;
    // switching to a different existing provider must not overwrite that
    // provider's stored URL.
    const editingCurrentProvider =
      !!subscription && trimmedProvider === subscription.provider_name;
    const mayUpdateProviderURL = !existingProvider || editingCurrentProvider;
    if (mayUpdateProviderURL && trimmedUrl) input.url = trimmedUrl;
    // Only send supplement fields the user deliberately changed. Omitting
    // them preserves vendor/manual values already stored by the backend.
    if (quotaDirty) {
      input.quota_bytes = quota !== null ? Math.round(quota * GB) : 0;
    }
    if (expiryDirty) {
      input.manual_expire = manualExpiryISO ?? '';
    }
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
              {existingProvider && (!subscription || providerName.trim() !== subscription.provider_name) && (
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
            <p className="text-xs text-muted-foreground">{t('subscriptions.pausedHint')}</p>
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
                onChange={(e) => {
                  setQuotaGB(e.target.value);
                  setQuotaDirty(true);
                }}
                placeholder={effectiveQuota ?? ''}
              />
              <p className="text-xs text-muted-foreground">
                {t('subscriptions.quotaGBHint')}
                {effectiveQuota
                  ? ` ${t('subscriptions.effectiveQuota', { value: effectiveQuota })}`
                  : ''}
              </p>
            </div>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                className="h-4 w-4 rounded border border-input"
                checked={resetConfigured}
                onChange={(e) => {
                  const checked = e.target.checked;
                  setResetConfigured(checked);
                  if (checked && (!resetDay || resetDay === '0')) setResetDay('1');
                }}
              />
              {t('subscriptions.resetConfigure')}
            </label>
            {!resetConfigured && (
              <p className="text-xs text-muted-foreground">{t('subscriptions.resetNoneHint')}</p>
            )}
            <div className={`grid grid-cols-3 gap-3 ${resetConfigured ? '' : 'opacity-60'}`}>
              <div className="space-y-1.5">
                <Label htmlFor="subscription-reset-day">{t('subscriptions.resetDay')}</Label>
                <Input
                  id="subscription-reset-day"
                  type="number"
                  min={1}
                  max={31}
                  disabled={!resetConfigured}
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
                  disabled={!resetConfigured}
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
                  disabled={!resetConfigured}
                  value={resetMinute}
                  onChange={(e) => setResetMinute(e.target.value)}
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="subscription-timezone">{t('subscriptions.timezone')}</Label>
              <Input
                id="subscription-timezone"
                disabled={!resetConfigured}
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
              <div className="grid grid-cols-2 gap-3">
                <Input
                  id="subscription-expiry"
                  type="datetime-local"
                  step={1}
                  value={manualExpiry}
                  onChange={(e) => {
                    setManualExpiry(e.target.value);
                    setExpiryDirty(true);
                  }}
                />
                <Input
                  id="subscription-expiry-timezone"
                  aria-label={t('subscriptions.manualExpiryTimezone')}
                  value={expiryTimezone}
                  list="subscription-expiry-timezones"
                  onChange={(e) => {
                    setExpiryTimezone(e.target.value);
                    setExpiryDirty(true);
                  }}
                  placeholder={t('subscriptions.serverTimezone')}
                />
              </div>
              <datalist id="subscription-expiry-timezones">
                {[...new Set([browserTimezone(), ...COMMON_TIMEZONES, expiryTimezone])].map((tz) => (
                  <option key={tz} value={tz} />
                ))}
              </datalist>
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
