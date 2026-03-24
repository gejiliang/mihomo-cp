import { useEffect, useState } from 'react';
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import type { Proxy } from '@/api/proxies';
import { useT } from '@/i18n';

type FieldType = 'text' | 'password' | 'number' | 'checkbox' | 'select';

interface FieldDef {
  key: string;
  label: string;
  type: FieldType;
  options?: string[];
}

const PROXY_TYPES = ['ss', 'trojan', 'vmess', 'vless', 'http', 'socks5', 'hysteria2', 'tuic'];

const PROTOCOL_FIELDS: Record<string, FieldDef[]> = {
  ss: [
    { key: 'cipher', label: 'proxy.cipher', type: 'select', options: ['aes-128-gcm', 'aes-256-gcm', 'chacha20-ietf-poly1305'] },
    { key: 'password', label: 'proxy.password', type: 'password' },
    { key: 'udp', label: 'proxy.udp', type: 'checkbox' },
  ],
  trojan: [
    { key: 'password', label: 'proxy.password', type: 'password' },
    { key: 'sni', label: 'proxy.sni', type: 'text' },
    { key: 'skip-cert-verify', label: 'proxy.skipCertVerify', type: 'checkbox' },
  ],
  vmess: [
    { key: 'uuid', label: 'proxy.uuid', type: 'text' },
    { key: 'alterId', label: 'proxy.alterId', type: 'number' },
    { key: 'cipher', label: 'proxy.cipher', type: 'select', options: ['auto', 'aes-128-gcm', 'chacha20-poly1305', 'none'] },
    { key: 'tls', label: 'proxy.tls', type: 'checkbox' },
    { key: 'servername', label: 'proxy.serverName', type: 'text' },
    { key: 'network', label: 'proxy.network', type: 'select', options: ['tcp', 'ws', 'h2', 'grpc'] },
  ],
  vless: [
    { key: 'uuid', label: 'proxy.uuid', type: 'text' },
    { key: 'flow', label: 'proxy.flow', type: 'select', options: ['', 'xtls-rprx-vision'] },
    { key: 'tls', label: 'proxy.tls', type: 'checkbox' },
    { key: 'servername', label: 'proxy.serverName', type: 'text' },
    { key: 'network', label: 'proxy.network', type: 'select', options: ['tcp', 'ws', 'h2', 'grpc'] },
  ],
  http: [
    { key: 'username', label: 'proxy.username', type: 'text' },
    { key: 'password', label: 'proxy.password', type: 'password' },
    { key: 'tls', label: 'proxy.tls', type: 'checkbox' },
  ],
  socks5: [
    { key: 'username', label: 'proxy.username', type: 'text' },
    { key: 'password', label: 'proxy.password', type: 'password' },
    { key: 'tls', label: 'proxy.tls', type: 'checkbox' },
    { key: 'udp', label: 'proxy.udp', type: 'checkbox' },
  ],
  hysteria2: [
    { key: 'password', label: 'proxy.password', type: 'password' },
    { key: 'obfs', label: 'proxy.obfs', type: 'select', options: ['', 'salamander'] },
    { key: 'obfs-password', label: 'proxy.obfsPassword', type: 'password' },
    { key: 'sni', label: 'proxy.sni', type: 'text' },
    { key: 'skip-cert-verify', label: 'proxy.skipCertVerify', type: 'checkbox' },
  ],
  tuic: [
    { key: 'uuid', label: 'proxy.uuid', type: 'text' },
    { key: 'password', label: 'proxy.password', type: 'password' },
    { key: 'congestion-controller', label: 'proxy.congestionController', type: 'select', options: ['cubic', 'bbr', 'new_reno'] },
    { key: 'sni', label: 'proxy.sni', type: 'text' },
    { key: 'skip-cert-verify', label: 'proxy.skipCertVerify', type: 'checkbox' },
  ],
};

interface ProxyFormProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  proxy?: Proxy;
  onSave: (data: { name: string; type: string; config: Record<string, unknown> }) => void;
}

interface FormState {
  name: string;
  type: string;
  server: string;
  port: string;
  config: Record<string, unknown>;
}

const DEFAULT_STATE: FormState = {
  name: '',
  type: 'ss',
  server: '',
  port: '',
  config: {},
};

export function ProxyForm({ open, onOpenChange, proxy, onSave }: ProxyFormProps) {
  const t = useT();
  const [form, setForm] = useState<FormState>(DEFAULT_STATE);

  useEffect(() => {
    if (open) {
      if (proxy) {
        setForm({
          name: proxy.name,
          type: proxy.type,
          server: String(proxy.config.server ?? ''),
          port: String(proxy.config.port ?? ''),
          config: { ...proxy.config },
        });
      } else {
        setForm(DEFAULT_STATE);
      }
    }
  }, [open, proxy]);

  const handleTypeChange = (newType: string) => {
    // Keep compatible fields when switching protocol
    const currentFields = PROTOCOL_FIELDS[form.type] ?? [];
    const newFields = PROTOCOL_FIELDS[newType] ?? [];
    const newFieldKeys = new Set(newFields.map((f) => f.key));

    const preservedConfig: Record<string, unknown> = {};
    for (const f of currentFields) {
      if (newFieldKeys.has(f.key) && form.config[f.key] !== undefined) {
        preservedConfig[f.key] = form.config[f.key];
      }
    }

    setForm((prev) => ({ ...prev, type: newType, config: preservedConfig }));
  };

  const setConfigField = (key: string, value: unknown) => {
    setForm((prev) => ({ ...prev, config: { ...prev.config, [key]: value } }));
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const config: Record<string, unknown> = {
      ...form.config,
      server: form.server,
      port: form.port ? Number(form.port) : undefined,
    };
    onSave({ name: form.name, type: form.type, config });
  };

  const fields = PROTOCOL_FIELDS[form.type] ?? [];

  const renderField = (field: FieldDef) => {
    const value = form.config[field.key];
    const label = t(field.label as Parameters<typeof t>[0]);

    if (field.type === 'checkbox') {
      return (
        <div key={field.key} className="flex items-center gap-2">
          <input
            id={`field-${field.key}`}
            type="checkbox"
            checked={Boolean(value)}
            onChange={(e) => setConfigField(field.key, e.target.checked)}
            className="h-4 w-4 rounded border border-input"
          />
          <Label htmlFor={`field-${field.key}`}>{label}</Label>
        </div>
      );
    }

    if (field.type === 'select' && field.options) {
      return (
        <div key={field.key} className="space-y-1.5">
          <Label htmlFor={`field-${field.key}`}>{label}</Label>
          <Select
            value={value !== undefined ? String(value) : ''}
            onValueChange={(v) => setConfigField(field.key, v ?? '')}
          >
            <SelectTrigger className="w-full">
              <SelectValue placeholder={label} />
            </SelectTrigger>
            <SelectContent>
              {field.options.map((opt) => (
                <SelectItem key={opt} value={opt}>
                  {opt || t('proxy.none')}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      );
    }

    if (field.type === 'number') {
      return (
        <div key={field.key} className="space-y-1.5">
          <Label htmlFor={`field-${field.key}`}>{label}</Label>
          <Input
            id={`field-${field.key}`}
            type="number"
            value={value !== undefined ? String(value) : ''}
            onChange={(e) => setConfigField(field.key, e.target.value ? Number(e.target.value) : '')}
          />
        </div>
      );
    }

    // text or password
    return (
      <div key={field.key} className="space-y-1.5">
        <Label htmlFor={`field-${field.key}`}>{label}</Label>
        <Input
          id={`field-${field.key}`}
          type={field.type === 'password' ? 'password' : 'text'}
          value={value !== undefined ? String(value) : ''}
          onChange={(e) => setConfigField(field.key, e.target.value)}
        />
      </div>
    );
  };

  return (
    <Dialog open={open} onOpenChange={(o) => onOpenChange(o)}>
      <DialogContent className="sm:max-w-lg overflow-y-auto max-h-[90vh]">
        <DialogHeader>
          <DialogTitle>{proxy ? t('proxies.editDialog') : t('proxies.addDialog')}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          {/* Common fields */}
          <div className="space-y-1.5">
            <Label htmlFor="proxy-name">{t('common.name')}</Label>
            <Input
              id="proxy-name"
              type="text"
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              placeholder={t('proxies.namePlaceholder')}
              required
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="proxy-type">{t('common.type')}</Label>
            <Select value={form.type} onValueChange={(v) => v && handleTypeChange(v)}>
              <SelectTrigger className="w-full">
                <SelectValue placeholder={t('proxies.selectType')} />
              </SelectTrigger>
              <SelectContent>
                {PROXY_TYPES.map((pt) => (
                  <SelectItem key={pt} value={pt}>
                    {pt}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="proxy-server">{t('proxies.server')}</Label>
            <Input
              id="proxy-server"
              type="text"
              value={form.server}
              onChange={(e) => setForm((p) => ({ ...p, server: e.target.value }))}
              placeholder={t('proxies.serverPlaceholder')}
              required
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="proxy-port">{t('proxies.port')}</Label>
            <Input
              id="proxy-port"
              type="number"
              value={form.port}
              onChange={(e) => setForm((p) => ({ ...p, port: e.target.value }))}
              placeholder={t('proxies.portPlaceholder')}
              required
            />
          </div>

          {/* Protocol-specific fields */}
          {fields.map(renderField)}

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
