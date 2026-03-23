import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { ChevronDownIcon, ChevronRightIcon } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { systemConfigApi } from '@/api/system-config';

// Helper to get nested value from flat-key paths like "dns.enable"
function getNestedValue(obj: Record<string, any>, path: string): any {
  const parts = path.split('.');
  let cur: any = obj;
  for (const p of parts) {
    if (cur == null) return undefined;
    cur = cur[p];
  }
  return cur;
}

// Helper to set nested value
function setNestedValue(obj: Record<string, any>, path: string, value: any): Record<string, any> {
  const parts = path.split('.');
  const result = { ...obj };
  let cur: Record<string, any> = result;
  for (let i = 0; i < parts.length - 1; i++) {
    const key = parts[i];
    cur[key] = cur[key] ? { ...cur[key] } : {};
    cur = cur[key];
  }
  cur[parts[parts.length - 1]] = value;
  return result;
}

interface CollapsibleSectionProps {
  title: string;
  children: React.ReactNode;
  defaultOpen?: boolean;
}

function CollapsibleSection({ title, children, defaultOpen = true }: CollapsibleSectionProps) {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <Card>
      <CardHeader
        className="cursor-pointer border-b select-none"
        onClick={() => setOpen((v) => !v)}
      >
        <div className="flex items-center gap-2">
          {open ? (
            <ChevronDownIcon className="h-4 w-4 text-muted-foreground" />
          ) : (
            <ChevronRightIcon className="h-4 w-4 text-muted-foreground" />
          )}
          <CardTitle>{title}</CardTitle>
        </div>
      </CardHeader>
      {open && <CardContent className="pt-4">{children}</CardContent>}
    </Card>
  );
}

interface FieldRowProps {
  label: string;
  htmlFor?: string;
  children: React.ReactNode;
}

function FieldRow({ label, htmlFor, children }: FieldRowProps) {
  return (
    <div className="grid grid-cols-[180px_1fr] items-start gap-4">
      <Label htmlFor={htmlFor} className="pt-2 text-right text-sm">
        {label}
      </Label>
      <div>{children}</div>
    </div>
  );
}

export default function SystemConfigPage() {
  const [config, setConfig] = useState<Record<string, any>>({});
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  const fetchConfig = useCallback(async () => {
    setLoading(true);
    try {
      const res = await systemConfigApi.get();
      setConfig(res.data ?? {});
    } catch {
      toast.error('Failed to load system config');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchConfig();
  }, [fetchConfig]);

  const get = (path: string) => getNestedValue(config, path);
  const set = (path: string, value: any) => setConfig((prev) => setNestedValue(prev, path, value));

  const handleSave = async () => {
    setSaving(true);
    try {
      await systemConfigApi.update(config);
      toast.success('System config saved');
    } catch {
      toast.error('Failed to save system config');
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div className="space-y-4">
        <h1 className="text-2xl font-bold">System Config</h1>
        <div className="text-center py-12 text-muted-foreground">Loading...</div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">System Config</h1>
        <Button onClick={handleSave} disabled={saving}>
          {saving ? 'Saving...' : 'Save'}
        </Button>
      </div>

      {/* General */}
      <CollapsibleSection title="General">
        <div className="space-y-4">
          <FieldRow label="mixed-port" htmlFor="cfg-mixed-port">
            <Input
              id="cfg-mixed-port"
              type="number"
              value={get('mixed-port') ?? ''}
              onChange={(e) => set('mixed-port', e.target.value ? Number(e.target.value) : undefined)}
              placeholder="7890"
            />
          </FieldRow>
          <FieldRow label="port" htmlFor="cfg-port">
            <Input
              id="cfg-port"
              type="number"
              value={get('port') ?? ''}
              onChange={(e) => set('port', e.target.value ? Number(e.target.value) : undefined)}
              placeholder="7891"
            />
          </FieldRow>
          <FieldRow label="socks-port" htmlFor="cfg-socks-port">
            <Input
              id="cfg-socks-port"
              type="number"
              value={get('socks-port') ?? ''}
              onChange={(e) => set('socks-port', e.target.value ? Number(e.target.value) : undefined)}
              placeholder="7892"
            />
          </FieldRow>
          <FieldRow label="allow-lan">
            <div className="flex items-center gap-2 pt-2">
              <input
                id="cfg-allow-lan"
                type="checkbox"
                checked={Boolean(get('allow-lan'))}
                onChange={(e) => set('allow-lan', e.target.checked)}
                className="h-4 w-4 rounded border border-input"
              />
              <Label htmlFor="cfg-allow-lan">Allow LAN connections</Label>
            </div>
          </FieldRow>
          <FieldRow label="bind-address" htmlFor="cfg-bind-address">
            <Input
              id="cfg-bind-address"
              type="text"
              value={get('bind-address') ?? ''}
              onChange={(e) => set('bind-address', e.target.value)}
              placeholder="*"
            />
          </FieldRow>
          <FieldRow label="mode" htmlFor="cfg-mode">
            <Select
              value={get('mode') ?? ''}
              onValueChange={(v) => set('mode', v)}
            >
              <SelectTrigger className="w-full" id="cfg-mode">
                <SelectValue placeholder="Select mode" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="rule">rule</SelectItem>
                <SelectItem value="global">global</SelectItem>
                <SelectItem value="direct">direct</SelectItem>
              </SelectContent>
            </Select>
          </FieldRow>
          <FieldRow label="log-level" htmlFor="cfg-log-level">
            <Select
              value={get('log-level') ?? ''}
              onValueChange={(v) => set('log-level', v)}
            >
              <SelectTrigger className="w-full" id="cfg-log-level">
                <SelectValue placeholder="Select log level" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="silent">silent</SelectItem>
                <SelectItem value="error">error</SelectItem>
                <SelectItem value="warning">warning</SelectItem>
                <SelectItem value="info">info</SelectItem>
                <SelectItem value="debug">debug</SelectItem>
              </SelectContent>
            </Select>
          </FieldRow>
          <FieldRow label="ipv6">
            <div className="flex items-center gap-2 pt-2">
              <input
                id="cfg-ipv6"
                type="checkbox"
                checked={Boolean(get('ipv6'))}
                onChange={(e) => set('ipv6', e.target.checked)}
                className="h-4 w-4 rounded border border-input"
              />
              <Label htmlFor="cfg-ipv6">Enable IPv6</Label>
            </div>
          </FieldRow>
        </div>
      </CollapsibleSection>

      {/* External Controller */}
      <CollapsibleSection title="External Controller">
        <div className="space-y-4">
          <FieldRow label="external-controller" htmlFor="cfg-ext-ctrl">
            <Input
              id="cfg-ext-ctrl"
              type="text"
              value={get('external-controller') ?? ''}
              onChange={(e) => set('external-controller', e.target.value)}
              placeholder="127.0.0.1:9090"
            />
          </FieldRow>
          <FieldRow label="secret" htmlFor="cfg-secret">
            <Input
              id="cfg-secret"
              type="password"
              value={get('secret') ?? ''}
              onChange={(e) => set('secret', e.target.value)}
              placeholder="API secret"
              autoComplete="new-password"
            />
          </FieldRow>
        </div>
      </CollapsibleSection>

      {/* TUN */}
      <CollapsibleSection title="TUN" defaultOpen={false}>
        <div className="space-y-4">
          <FieldRow label="tun.enable">
            <div className="flex items-center gap-2 pt-2">
              <input
                id="cfg-tun-enable"
                type="checkbox"
                checked={Boolean(get('tun.enable'))}
                onChange={(e) => set('tun.enable', e.target.checked)}
                className="h-4 w-4 rounded border border-input"
              />
              <Label htmlFor="cfg-tun-enable">Enable TUN</Label>
            </div>
          </FieldRow>
          <FieldRow label="tun.stack" htmlFor="cfg-tun-stack">
            <Select
              value={get('tun.stack') ?? ''}
              onValueChange={(v) => set('tun.stack', v)}
            >
              <SelectTrigger className="w-full" id="cfg-tun-stack">
                <SelectValue placeholder="Select stack" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="system">system</SelectItem>
                <SelectItem value="gvisor">gvisor</SelectItem>
                <SelectItem value="mixed">mixed</SelectItem>
              </SelectContent>
            </Select>
          </FieldRow>
          <FieldRow label="tun.dns-hijack" htmlFor="cfg-tun-dns-hijack">
            <Input
              id="cfg-tun-dns-hijack"
              type="text"
              value={
                Array.isArray(get('tun.dns-hijack'))
                  ? (get('tun.dns-hijack') as string[]).join(', ')
                  : (get('tun.dns-hijack') ?? '')
              }
              onChange={(e) =>
                set(
                  'tun.dns-hijack',
                  e.target.value ? e.target.value.split(',').map((s) => s.trim()) : []
                )
              }
              placeholder="any:53, tcp://any:53"
            />
          </FieldRow>
          <FieldRow label="tun.auto-route">
            <div className="flex items-center gap-2 pt-2">
              <input
                id="cfg-tun-auto-route"
                type="checkbox"
                checked={Boolean(get('tun.auto-route'))}
                onChange={(e) => set('tun.auto-route', e.target.checked)}
                className="h-4 w-4 rounded border border-input"
              />
              <Label htmlFor="cfg-tun-auto-route">Auto Route</Label>
            </div>
          </FieldRow>
        </div>
      </CollapsibleSection>

      {/* DNS */}
      <CollapsibleSection title="DNS" defaultOpen={false}>
        <div className="space-y-4">
          <FieldRow label="dns.enable">
            <div className="flex items-center gap-2 pt-2">
              <input
                id="cfg-dns-enable"
                type="checkbox"
                checked={Boolean(get('dns.enable'))}
                onChange={(e) => set('dns.enable', e.target.checked)}
                className="h-4 w-4 rounded border border-input"
              />
              <Label htmlFor="cfg-dns-enable">Enable DNS</Label>
            </div>
          </FieldRow>
          <FieldRow label="dns.listen" htmlFor="cfg-dns-listen">
            <Input
              id="cfg-dns-listen"
              type="text"
              value={get('dns.listen') ?? ''}
              onChange={(e) => set('dns.listen', e.target.value)}
              placeholder="0.0.0.0:53"
            />
          </FieldRow>
          <FieldRow label="dns.enhanced-mode" htmlFor="cfg-dns-mode">
            <Select
              value={get('dns.enhanced-mode') ?? ''}
              onValueChange={(v) => set('dns.enhanced-mode', v)}
            >
              <SelectTrigger className="w-full" id="cfg-dns-mode">
                <SelectValue placeholder="Select mode" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="fake-ip">fake-ip</SelectItem>
                <SelectItem value="redir-host">redir-host</SelectItem>
              </SelectContent>
            </Select>
          </FieldRow>
          <FieldRow label="dns.nameserver" htmlFor="cfg-dns-nameserver">
            <Textarea
              id="cfg-dns-nameserver"
              value={
                Array.isArray(get('dns.nameserver'))
                  ? (get('dns.nameserver') as string[]).join('\n')
                  : (get('dns.nameserver') ?? '')
              }
              onChange={(e) =>
                set(
                  'dns.nameserver',
                  e.target.value ? e.target.value.split('\n').map((s) => s.trim()).filter(Boolean) : []
                )
              }
              placeholder="8.8.8.8&#10;1.1.1.1"
              rows={3}
            />
          </FieldRow>
          <FieldRow label="dns.fallback" htmlFor="cfg-dns-fallback">
            <Textarea
              id="cfg-dns-fallback"
              value={
                Array.isArray(get('dns.fallback'))
                  ? (get('dns.fallback') as string[]).join('\n')
                  : (get('dns.fallback') ?? '')
              }
              onChange={(e) =>
                set(
                  'dns.fallback',
                  e.target.value ? e.target.value.split('\n').map((s) => s.trim()).filter(Boolean) : []
                )
              }
              placeholder="8.8.4.4&#10;1.0.0.1"
              rows={3}
            />
          </FieldRow>
        </div>
      </CollapsibleSection>

      <div className="flex justify-end">
        <Button onClick={handleSave} disabled={saving} size="lg">
          {saving ? 'Saving...' : 'Save Configuration'}
        </Button>
      </div>
    </div>
  );
}
