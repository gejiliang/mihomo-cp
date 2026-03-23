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
    { key: 'cipher', label: 'Cipher', type: 'select', options: ['aes-128-gcm', 'aes-256-gcm', 'chacha20-ietf-poly1305'] },
    { key: 'password', label: 'Password', type: 'password' },
    { key: 'udp', label: 'UDP', type: 'checkbox' },
  ],
  trojan: [
    { key: 'password', label: 'Password', type: 'password' },
    { key: 'sni', label: 'SNI', type: 'text' },
    { key: 'skip-cert-verify', label: 'Skip Cert Verify', type: 'checkbox' },
  ],
  vmess: [
    { key: 'uuid', label: 'UUID', type: 'text' },
    { key: 'alterId', label: 'Alter ID', type: 'number' },
    { key: 'cipher', label: 'Cipher', type: 'select', options: ['auto', 'aes-128-gcm', 'chacha20-poly1305', 'none'] },
    { key: 'tls', label: 'TLS', type: 'checkbox' },
    { key: 'servername', label: 'Server Name', type: 'text' },
    { key: 'network', label: 'Network', type: 'select', options: ['tcp', 'ws', 'h2', 'grpc'] },
  ],
  vless: [
    { key: 'uuid', label: 'UUID', type: 'text' },
    { key: 'flow', label: 'Flow', type: 'select', options: ['', 'xtls-rprx-vision'] },
    { key: 'tls', label: 'TLS', type: 'checkbox' },
    { key: 'servername', label: 'Server Name', type: 'text' },
    { key: 'network', label: 'Network', type: 'select', options: ['tcp', 'ws', 'h2', 'grpc'] },
  ],
  http: [
    { key: 'username', label: 'Username', type: 'text' },
    { key: 'password', label: 'Password', type: 'password' },
    { key: 'tls', label: 'TLS', type: 'checkbox' },
  ],
  socks5: [
    { key: 'username', label: 'Username', type: 'text' },
    { key: 'password', label: 'Password', type: 'password' },
    { key: 'tls', label: 'TLS', type: 'checkbox' },
    { key: 'udp', label: 'UDP', type: 'checkbox' },
  ],
  hysteria2: [
    { key: 'password', label: 'Password', type: 'password' },
    { key: 'obfs', label: 'Obfs', type: 'select', options: ['', 'salamander'] },
    { key: 'obfs-password', label: 'Obfs Password', type: 'password' },
    { key: 'sni', label: 'SNI', type: 'text' },
    { key: 'skip-cert-verify', label: 'Skip Cert Verify', type: 'checkbox' },
  ],
  tuic: [
    { key: 'uuid', label: 'UUID', type: 'text' },
    { key: 'password', label: 'Password', type: 'password' },
    { key: 'congestion-controller', label: 'Congestion Controller', type: 'select', options: ['cubic', 'bbr', 'new_reno'] },
    { key: 'sni', label: 'SNI', type: 'text' },
    { key: 'skip-cert-verify', label: 'Skip Cert Verify', type: 'checkbox' },
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
          <Label htmlFor={`field-${field.key}`}>{field.label}</Label>
        </div>
      );
    }

    if (field.type === 'select' && field.options) {
      return (
        <div key={field.key} className="space-y-1.5">
          <Label htmlFor={`field-${field.key}`}>{field.label}</Label>
          <Select
            value={value !== undefined ? String(value) : ''}
            onValueChange={(v) => setConfigField(field.key, v ?? '')}
          >
            <SelectTrigger className="w-full">
              <SelectValue placeholder={`Select ${field.label}`} />
            </SelectTrigger>
            <SelectContent>
              {field.options.map((opt) => (
                <SelectItem key={opt} value={opt}>
                  {opt || '(none)'}
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
          <Label htmlFor={`field-${field.key}`}>{field.label}</Label>
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
        <Label htmlFor={`field-${field.key}`}>{field.label}</Label>
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
          <DialogTitle>{proxy ? 'Edit Proxy' : 'Add Proxy'}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          {/* Common fields */}
          <div className="space-y-1.5">
            <Label htmlFor="proxy-name">Name</Label>
            <Input
              id="proxy-name"
              type="text"
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              placeholder="Proxy name"
              required
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="proxy-type">Type</Label>
            <Select value={form.type} onValueChange={(v) => v && handleTypeChange(v)}>
              <SelectTrigger className="w-full">
                <SelectValue placeholder="Select type" />
              </SelectTrigger>
              <SelectContent>
                {PROXY_TYPES.map((t) => (
                  <SelectItem key={t} value={t}>
                    {t}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="proxy-server">Server</Label>
            <Input
              id="proxy-server"
              type="text"
              value={form.server}
              onChange={(e) => setForm((p) => ({ ...p, server: e.target.value }))}
              placeholder="hostname or IP"
              required
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="proxy-port">Port</Label>
            <Input
              id="proxy-port"
              type="number"
              value={form.port}
              onChange={(e) => setForm((p) => ({ ...p, port: e.target.value }))}
              placeholder="1-65535"
              required
            />
          </div>

          {/* Protocol-specific fields */}
          {fields.map(renderField)}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit">Save</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
