import { useCallback, useEffect, useMemo, useState } from 'react';
import { toast } from 'sonner';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  AlertTriangleIcon,
  BellIcon,
  GaugeIcon,
  PencilIcon,
  PlusIcon,
  RefreshCwIcon,
  Trash2Icon,
} from 'lucide-react';
import {
  trafficApi,
  formatBytes,
  GB,
  type ProviderSummary,
  type TrafficSummary,
  type DailyUsage,
  type TrafficAlert,
  type TrafficSettings,
  type TrafficProvider,
  type ProviderInput,
} from '@/api/traffic';
import { useT } from '@/i18n';

// Categorical series colors (validated palette, slots 1-2 then 3, 5).
const SERIES_LIGHT = ['#2a78d6', '#1baf7a', '#eda100', '#4a3aa7'];
const SERIES_DARK = ['#3987e5', '#199e70', '#c98500', '#9085e9'];
const UNASSIGNED_COLOR = '#898781';

function useIsDark() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains('dark'));
  useEffect(() => {
    const observer = new MutationObserver(() =>
      setDark(document.documentElement.classList.contains('dark'))
    );
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
    return () => observer.disconnect();
  }, []);
  return dark;
}

// Status colors for the quota meter (icon + label always accompany them).
function meterColor(percent: number): string {
  if (percent >= 95) return '#d03b3b';
  if (percent >= 80) return '#fab219';
  return '#2a78d6';
}

function ProviderCard({
  summary,
  onEdit,
}: {
  summary: ProviderSummary;
  onEdit: (p: TrafficProvider) => void;
}) {
  const t = useT();
  const p = summary.provider;
  const pct = Math.min(summary.percent, 100);
  const over = summary.percent >= 80;

  return (
    <Card>
      <CardHeader className="pb-3">
        <div className="flex items-center justify-between">
          <CardTitle className="text-base flex items-center gap-2">
            {p.name}
            {!p.enabled && <Badge variant="secondary">{t('traffic.disabled')}</Badge>}
          </CardTitle>
          <Button variant="ghost" size="sm" onClick={() => onEdit(p)}>
            <PencilIcon className="h-4 w-4" />
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          {t('traffic.cycle')}: {summary.cycle_start} ~ {summary.cycle_end}（{t('traffic.resetDay')}{' '}
          {p.reset_day}）
        </p>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex items-baseline justify-between">
          <span className="text-2xl font-semibold">
            {summary.percent.toFixed(1)}%
            {over && (
              <AlertTriangleIcon
                className="inline-block h-5 w-5 ml-2 align-baseline"
                style={{ color: meterColor(summary.percent) }}
              />
            )}
          </span>
          <span className="text-sm text-muted-foreground">
            {formatBytes(summary.usage_bytes)} / {formatBytes(p.quota_bytes)}
          </span>
        </div>
        <div className="h-2 rounded-full bg-muted overflow-hidden">
          <div
            className="h-full rounded-full transition-all"
            style={{ width: `${pct}%`, backgroundColor: meterColor(summary.percent) }}
          />
        </div>
        <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
          <span className="text-muted-foreground">{t('traffic.localCounted')}</span>
          <span className="text-right tabular-nums">{formatBytes(summary.local.total)}</span>
          <span className="text-muted-foreground">{t('traffic.vendorReported')}</span>
          <span className="text-right tabular-nums">
            {summary.reported
              ? formatBytes(summary.reported.upload + summary.reported.download)
              : '—'}
          </span>
          <span className="text-muted-foreground">{t('traffic.today')}</span>
          <span className="text-right tabular-nums">{formatBytes(summary.today_bytes)}</span>
          <span className="text-muted-foreground">{t('traffic.estDaysLeft')}</span>
          <span className="text-right tabular-nums">
            {summary.est_days_left >= 0 ? `≈ ${summary.est_days_left.toFixed(1)} ${t('traffic.days')}` : '—'}
          </span>
        </div>
        {summary.top_nodes.length > 0 && (
          <div className="pt-1">
            <p className="text-xs text-muted-foreground mb-1">{t('traffic.topNodes')}</p>
            <div className="space-y-0.5">
              {summary.top_nodes.slice(0, 5).map((n) => (
                <div key={n.node} className="flex justify-between text-xs">
                  <span className="truncate">{n.node}</span>
                  <span className="text-muted-foreground tabular-nums ml-2">
                    {formatBytes(n.total)}
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function DailyChart({
  history,
  providers,
  isDark,
}: {
  history: DailyUsage[];
  providers: TrafficProvider[];
  isDark: boolean;
}) {
  const t = useT();
  const series = isDark ? SERIES_DARK : SERIES_LIGHT;
  const colorOf = useMemo(() => {
    const m = new Map<string, string>();
    providers.forEach((p, i) => m.set(p.id, series[i % series.length]));
    return m;
  }, [providers, series]);

  const max = Math.max(
    1,
    ...history.map((d) => Object.values(d.providers).reduce((a, b) => a + b, 0) + d.unassigned)
  );
  const hasUnassigned = history.some((d) => d.unassigned > 0);

  return (
    <div>
      <div className="flex items-center gap-4 mb-3 text-xs">
        {providers.map((p) => (
          <span key={p.id} className="flex items-center gap-1.5">
            <span
              className="inline-block w-2.5 h-2.5 rounded-sm"
              style={{ backgroundColor: colorOf.get(p.id) }}
            />
            {p.name}
          </span>
        ))}
        {hasUnassigned && (
          <span className="flex items-center gap-1.5">
            <span
              className="inline-block w-2.5 h-2.5 rounded-sm"
              style={{ backgroundColor: UNASSIGNED_COLOR }}
            />
            {t('traffic.unassigned')}
          </span>
        )}
      </div>
      <div className="overflow-x-auto">
        <div className="flex items-end gap-[3px] h-40 min-w-[560px]">
          {history.map((d) => {
            const total = Object.values(d.providers).reduce((a, b) => a + b, 0) + d.unassigned;
            return (
              <div key={d.date} className="relative group flex-1 h-full flex flex-col justify-end">
                <div className="flex flex-col-reverse gap-[2px]">
                  {providers.map((p) => {
                    const v = d.providers[p.id] || 0;
                    if (v <= 0) return null;
                    return (
                      <div
                        key={p.id}
                        className="w-full rounded-[2px]"
                        style={{
                          height: `${Math.max((v / max) * 152, 2)}px`,
                          backgroundColor: colorOf.get(p.id),
                        }}
                      />
                    );
                  })}
                  {d.unassigned > 0 && (
                    <div
                      className="w-full rounded-[2px]"
                      style={{
                        height: `${Math.max((d.unassigned / max) * 152, 2)}px`,
                        backgroundColor: UNASSIGNED_COLOR,
                      }}
                    />
                  )}
                </div>
                <div className="hidden group-hover:block absolute bottom-full left-1/2 -translate-x-1/2 mb-1 z-10 rounded-md border bg-popover text-popover-foreground shadow-md px-2.5 py-1.5 text-xs whitespace-nowrap">
                  <p className="font-medium mb-0.5">{d.date}</p>
                  {providers.map((p) => (
                    <p key={p.id} className="flex items-center gap-1.5">
                      <span
                        className="inline-block w-2 h-2 rounded-sm"
                        style={{ backgroundColor: colorOf.get(p.id) }}
                      />
                      {p.name}: {formatBytes(d.providers[p.id] || 0)}
                    </p>
                  ))}
                  {d.unassigned > 0 && (
                    <p className="flex items-center gap-1.5">
                      <span
                        className="inline-block w-2 h-2 rounded-sm"
                        style={{ backgroundColor: UNASSIGNED_COLOR }}
                      />
                      {t('traffic.unassigned')}: {formatBytes(d.unassigned)}
                    </p>
                  )}
                  <p className="text-muted-foreground mt-0.5">
                    {t('traffic.total')}: {formatBytes(total)}
                  </p>
                </div>
              </div>
            );
          })}
        </div>
        <div className="flex gap-[3px] min-w-[560px] mt-1">
          {history.map((d, i) => (
            <div key={d.date} className="flex-1 text-center">
              {(i === 0 || d.date.endsWith('01') || i === history.length - 1) && (
                <span className="text-[10px] text-muted-foreground">{d.date.slice(5)}</span>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

const EMPTY_FORM = {
  name: '',
  quotaGB: '',
  resetDay: '1',
  resetHour: '0',
  resetMinute: '0',
  resetTimezone: '',
  patterns: '',
  subscriptionUrl: '',
  enabled: true,
};

export default function TrafficPage() {
  const t = useT();
  const isDark = useIsDark();

  const [summary, setSummary] = useState<TrafficSummary | null>(null);
  const [history, setHistory] = useState<DailyUsage[]>([]);
  const [alerts, setAlerts] = useState<TrafficAlert[]>([]);
  const [settings, setSettings] = useState<TrafficSettings | null>(null);
  const [loading, setLoading] = useState(true);

  // provider dialog
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<TrafficProvider | null>(null);
  const [form, setForm] = useState(EMPTY_FORM);
  const [saving, setSaving] = useState(false);

  // settings form
  const [channel, setChannel] = useState('');
  const [target, setTarget] = useState('');
  const [thresholds, setThresholds] = useState('80,95');
  const [testing, setTesting] = useState(false);

  const fetchAll = useCallback(async () => {
    try {
      const [s, h, a, st] = await Promise.all([
        trafficApi.summary(),
        trafficApi.history(30),
        trafficApi.alerts(20),
        trafficApi.settings(),
      ]);
      setSummary(s.data);
      setHistory(h.data ?? []);
      setAlerts(a.data ?? []);
      setSettings(st.data);
      setChannel(st.data.alert_channel);
      setTarget(st.data.alert_target);
      try {
        const arr = JSON.parse(st.data.alert_thresholds);
        if (Array.isArray(arr)) setThresholds(arr.join(','));
      } catch {
        /* keep default */
      }
    } catch {
      toast.error(t('traffic.loadFailed'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchAll();
    const timer = setInterval(fetchAll, 60_000);
    return () => clearInterval(timer);
  }, [fetchAll]);

  const providers = useMemo(
    () => (summary ? summary.providers.map((ps) => ps.provider) : []),
    [summary]
  );
  const providerName = useCallback(
    (id: string) => providers.find((p) => p.id === id)?.name ?? id,
    [providers]
  );

  const openCreate = () => {
    setEditing(null);
    setForm(EMPTY_FORM);
    setDialogOpen(true);
  };
  const openEdit = (p: TrafficProvider) => {
    setEditing(p);
    setForm({
      name: p.name,
      quotaGB: p.quota_bytes > 0 ? String(Math.round((p.quota_bytes / GB) * 100) / 100) : '',
      resetDay: String(p.reset_day),
      resetHour: String(p.reset_hour ?? 0),
      resetMinute: String(p.reset_minute ?? 0),
      resetTimezone: p.reset_timezone ?? '',
      patterns: p.node_patterns.join(', '),
      subscriptionUrl: p.subscription_url,
      enabled: p.enabled,
    });
    setDialogOpen(true);
  };

  const saveProvider = async () => {
    const quotaGB = parseFloat(form.quotaGB);
    const resetDay = parseInt(form.resetDay, 10);
    const resetHour = parseInt(form.resetHour, 10);
    const resetMinute = parseInt(form.resetMinute, 10);
    const timezone = form.resetTimezone.trim();
    let validTimezone = true;
    try {
      if (timezone) new Intl.DateTimeFormat('en-US', { timeZone: timezone });
    } catch {
      validTimezone = false;
    }
    if (
      !form.name.trim() ||
      isNaN(quotaGB) ||
      quotaGB < 0 ||
      isNaN(resetDay) ||
      resetDay < 1 ||
      resetDay > 31 ||
      isNaN(resetHour) ||
      resetHour < 0 ||
      resetHour > 23 ||
      isNaN(resetMinute) ||
      resetMinute < 0 ||
      resetMinute > 59 ||
      !validTimezone
    ) {
      toast.error(t('traffic.formInvalid'));
      return;
    }
    const input: ProviderInput = {
      name: form.name.trim(),
      quota_bytes: Math.round(quotaGB * GB),
      reset_day: resetDay,
      reset_hour: resetHour,
      reset_minute: resetMinute,
      reset_timezone: timezone,
      node_patterns: form.patterns
        .split(/[,，]/)
        .map((s) => s.trim())
        .filter(Boolean),
      subscription_url: form.subscriptionUrl.trim(),
      enabled: form.enabled,
    };
    setSaving(true);
    try {
      if (editing) {
        await trafficApi.updateProvider(editing.id, input);
      } else {
        await trafficApi.createProvider(input);
      }
      toast.success(t('common.save') + ' OK');
      setDialogOpen(false);
      fetchAll();
    } catch {
      toast.error(t('traffic.saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  const deleteProvider = async () => {
    if (!editing) return;
    if (!window.confirm(t('traffic.deleteConfirm'))) return;
    try {
      await trafficApi.deleteProvider(editing.id);
      toast.success(t('common.delete') + ' OK');
      setDialogOpen(false);
      fetchAll();
    } catch {
      toast.error(t('traffic.saveFailed'));
    }
  };

  const saveSettings = async () => {
    const arr = thresholds
      .split(/[,，]/)
      .map((s) => parseInt(s.trim(), 10))
      .filter((n) => !isNaN(n) && n > 0 && n <= 100);
    try {
      await trafficApi.updateSettings({
        alert_channel: channel,
        alert_target: target.trim(),
        alert_thresholds: JSON.stringify(arr.length ? arr : [80, 95]),
      });
      toast.success(t('traffic.settingsSaved'));
      fetchAll();
    } catch {
      toast.error(t('traffic.saveFailed'));
    }
  };

  const testNotify = async () => {
    setTesting(true);
    try {
      await trafficApi.testNotify();
      toast.success(t('traffic.testSent'));
    } catch {
      toast.error(t('traffic.testFailed'));
    } finally {
      setTesting(false);
    }
  };

  if (loading) {
    return <p className="text-muted-foreground">{t('common.loading')}</p>;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold flex items-center gap-2">
          <GaugeIcon className="h-6 w-6" />
          {t('nav.traffic')}
        </h1>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={fetchAll}>
            <RefreshCwIcon className="h-4 w-4 mr-1" />
            {t('common.refresh')}
          </Button>
          <Button size="sm" onClick={openCreate}>
            <PlusIcon className="h-4 w-4 mr-1" />
            {t('traffic.addProvider')}
          </Button>
        </div>
      </div>

      {summary && summary.providers.length === 0 && (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            {t('traffic.noProviders')}
          </CardContent>
        </Card>
      )}

      <div className="grid gap-4 md:grid-cols-2">
        {summary?.providers.map((ps) => (
          <ProviderCard key={ps.provider.id} summary={ps} onEdit={openEdit} />
        ))}
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('traffic.dailyUsage')}</CardTitle>
        </CardHeader>
        <CardContent>
          <DailyChart history={history} providers={providers} isDark={isDark} />
        </CardContent>
      </Card>

      {summary && summary.unassigned.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t('traffic.unassignedNodes')}</CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('common.name')}</TableHead>
                  <TableHead className="text-right">{t('traffic.upload')}</TableHead>
                  <TableHead className="text-right">{t('traffic.download')}</TableHead>
                  <TableHead className="text-right">{t('traffic.total')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {summary.unassigned.slice(0, 15).map((n) => (
                  <TableRow key={n.node}>
                    <TableCell>{n.node}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatBytes(n.upload)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatBytes(n.download)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatBytes(n.total)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <BellIcon className="h-4 w-4" />
              {t('traffic.alertSettings')}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-1.5">
              <Label>{t('traffic.alertChannel')}</Label>
              <select
                className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm"
                value={channel}
                onChange={(e) => setChannel(e.target.value)}
              >
                <option value="">{t('traffic.channelNone')}</option>
                <option value="bark">Bark</option>
                <option value="ntfy">ntfy</option>
                <option value="webhook">Webhook (JSON POST)</option>
              </select>
            </div>
            {channel && (
              <div className="space-y-1.5">
                <Label>{t('traffic.alertTarget')}</Label>
                <Input
                  value={target}
                  onChange={(e) => setTarget(e.target.value)}
                  placeholder={
                    channel === 'bark'
                      ? 'https://api.day.app/<device-key>'
                      : channel === 'ntfy'
                        ? 'https://ntfy.sh/<topic>'
                        : 'https://example.com/webhook'
                  }
                />
              </div>
            )}
            <div className="space-y-1.5">
              <Label>{t('traffic.alertThresholds')}</Label>
              <Input
                value={thresholds}
                onChange={(e) => setThresholds(e.target.value)}
                placeholder="80,95"
              />
            </div>
            <div className="flex gap-2 pt-1">
              <Button size="sm" onClick={saveSettings}>
                {t('common.save')}
              </Button>
              <Button size="sm" variant="outline" onClick={testNotify} disabled={testing || !channel}>
                {t('traffic.testNotify')}
              </Button>
            </div>
            {settings && (
              <p className="text-xs text-muted-foreground">{t('traffic.alertHint')}</p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t('traffic.recentAlerts')}</CardTitle>
          </CardHeader>
          <CardContent>
            {alerts.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t('traffic.noAlerts')}</p>
            ) : (
              <div className="space-y-2">
                {alerts.map((a) => (
                  <div key={a.id} className="text-sm border-l-2 pl-3 py-0.5" style={{ borderColor: meterColor(a.threshold) }}>
                    <p>{a.message}</p>
                    <p className="text-xs text-muted-foreground">
                      {providerName(a.provider_id)} · {new Date(a.created_at).toLocaleString()}
                      {!a.notified && ` · ${t('traffic.notNotified')}`}
                    </p>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {editing ? t('traffic.editProvider') : t('traffic.addProvider')}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label>{t('common.name')}</Label>
              <Input
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder="PS"
              />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label>{t('traffic.quotaGB')}</Label>
                <Input
                  value={form.quotaGB}
                  onChange={(e) => setForm({ ...form, quotaGB: e.target.value })}
                  placeholder="1024"
                />
              </div>
              <div className="space-y-1.5">
                <Label>{t('traffic.resetDayLabel')}</Label>
                <Input
                  value={form.resetDay}
                  onChange={(e) => setForm({ ...form, resetDay: e.target.value })}
                  placeholder="20"
                />
              </div>
            </div>
            <div className="grid grid-cols-3 gap-3">
              <div className="space-y-1.5">
                <Label>{t('traffic.resetHour')}</Label>
                <Input
                  type="number"
                  min={0}
                  max={23}
                  value={form.resetHour}
                  onChange={(e) => setForm({ ...form, resetHour: e.target.value })}
                  placeholder="0"
                />
              </div>
              <div className="space-y-1.5">
                <Label>{t('traffic.resetMinute')}</Label>
                <Input
                  type="number"
                  min={0}
                  max={59}
                  value={form.resetMinute}
                  onChange={(e) => setForm({ ...form, resetMinute: e.target.value })}
                  placeholder="0"
                />
              </div>
              <div className="space-y-1.5">
                <Label>{t('traffic.resetTimezone')}</Label>
                <Input
                  value={form.resetTimezone}
                  onChange={(e) => setForm({ ...form, resetTimezone: e.target.value })}
                  placeholder="Asia/Shanghai"
                />
              </div>
            </div>
            <p className="text-xs text-muted-foreground">{t('traffic.resetTimezoneHint')}</p>
            <div className="space-y-1.5">
              <Label>{t('traffic.nodePatterns')}</Label>
              <Input
                value={form.patterns}
                onChange={(e) => setForm({ ...form, patterns: e.target.value })}
                placeholder="PS-*, HK-*"
              />
              <p className="text-xs text-muted-foreground">{t('traffic.nodePatternsHint')}</p>
            </div>
            <div className="space-y-1.5">
              <Label>{t('traffic.subscriptionUrl')}</Label>
              <Input
                value={form.subscriptionUrl}
                onChange={(e) => setForm({ ...form, subscriptionUrl: e.target.value })}
                placeholder="https://..."
              />
              <p className="text-xs text-muted-foreground">{t('traffic.subscriptionUrlHint')}</p>
            </div>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={form.enabled}
                onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
              />
              {t('traffic.enabled')}
            </label>
          </div>
          <DialogFooter className="flex items-center justify-between sm:justify-between">
            <div>
              {editing && (
                <Button variant="destructive" size="sm" onClick={deleteProvider}>
                  <Trash2Icon className="h-4 w-4 mr-1" />
                  {t('common.delete')}
                </Button>
              )}
            </div>
            <div className="flex gap-2">
              <Button variant="outline" onClick={() => setDialogOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button onClick={saveProvider} disabled={saving}>
                {saving ? t('common.saving') : t('common.save')}
              </Button>
            </div>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
