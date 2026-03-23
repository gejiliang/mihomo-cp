import { useEffect, useState } from 'react';
import { toast } from 'sonner';
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
import type { Rule } from '@/api/rules';
import { proxyGroupsApi } from '@/api/proxy-groups';

const RULE_TYPES = [
  'DOMAIN',
  'DOMAIN-SUFFIX',
  'DOMAIN-KEYWORD',
  'IP-CIDR',
  'IP-CIDR6',
  'SRC-IP-CIDR',
  'DST-PORT',
  'PROCESS-NAME',
  'GEOIP',
  'RULE-SET',
  'MATCH',
];

const NO_RESOLVE_TYPES = ['IP-CIDR', 'IP-CIDR6', 'SRC-IP-CIDR'];
const NO_PAYLOAD_TYPES = ['MATCH'];

const STATIC_TARGETS = ['DIRECT', 'REJECT'];

interface RuleFormProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  rule?: Rule;
  onSave: (data: Partial<Rule>) => void;
}

interface FormState {
  type: string;
  payload: string;
  target: string;
  noResolve: boolean;
}

const DEFAULT_STATE: FormState = {
  type: 'DOMAIN',
  payload: '',
  target: 'DIRECT',
  noResolve: false,
};

export function RuleForm({ open, onOpenChange, rule, onSave }: RuleFormProps) {
  const [form, setForm] = useState<FormState>(DEFAULT_STATE);
  const [groupNames, setGroupNames] = useState<string[]>([]);

  useEffect(() => {
    proxyGroupsApi.list().then((res) => {
      setGroupNames((res.data ?? []).map((g) => g.name));
    }).catch(() => {
      toast.error('Failed to load proxy groups');
    });
  }, []);

  useEffect(() => {
    if (open) {
      if (rule) {
        setForm({
          type: rule.type,
          payload: rule.payload ?? '',
          target: rule.target ?? 'DIRECT',
          noResolve: Boolean(rule.params?.['no-resolve']),
        });
      } else {
        setForm(DEFAULT_STATE);
      }
    }
  }, [open, rule]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const params: Record<string, any> = {};
    if (NO_RESOLVE_TYPES.includes(form.type) && form.noResolve) {
      params['no-resolve'] = true;
    }
    onSave({
      type: form.type,
      payload: NO_PAYLOAD_TYPES.includes(form.type) ? '' : form.payload,
      target: form.target,
      params,
    });
  };

  const allTargets = [...STATIC_TARGETS, ...groupNames];
  const showPayload = !NO_PAYLOAD_TYPES.includes(form.type);
  const showNoResolve = NO_RESOLVE_TYPES.includes(form.type);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{rule ? 'Edit Rule' : 'Add Rule'}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="rule-type">Type</Label>
            <Select
              value={form.type}
              onValueChange={(v) => v && setForm((p) => ({ ...p, type: v, payload: '' }))}
            >
              <SelectTrigger className="w-full" id="rule-type">
                <SelectValue placeholder="Select type" />
              </SelectTrigger>
              <SelectContent>
                {RULE_TYPES.map((t) => (
                  <SelectItem key={t} value={t}>
                    {t}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {showPayload && (
            <div className="space-y-1.5">
              <Label htmlFor="rule-payload">Payload</Label>
              <Input
                id="rule-payload"
                type="text"
                value={form.payload}
                onChange={(e) => setForm((p) => ({ ...p, payload: e.target.value }))}
                placeholder="e.g. example.com, 192.168.1.0/24"
                required
              />
            </div>
          )}

          <div className="space-y-1.5">
            <Label htmlFor="rule-target">Target</Label>
            <Select
              value={form.target}
              onValueChange={(v) => v && setForm((p) => ({ ...p, target: v }))}
            >
              <SelectTrigger className="w-full" id="rule-target">
                <SelectValue placeholder="Select target" />
              </SelectTrigger>
              <SelectContent>
                {allTargets.map((t) => (
                  <SelectItem key={t} value={t}>
                    {t}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {showNoResolve && (
            <div className="flex items-center gap-2">
              <input
                id="rule-no-resolve"
                type="checkbox"
                checked={form.noResolve}
                onChange={(e) => setForm((p) => ({ ...p, noResolve: e.target.checked }))}
                className="h-4 w-4 rounded border border-input"
              />
              <Label htmlFor="rule-no-resolve">No Resolve</Label>
            </div>
          )}

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
