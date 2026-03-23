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
import { GroupMemberList } from './group-member-list';
import type { ProxyGroup } from '@/api/proxy-groups';

const GROUP_TYPES = ['select', 'fallback', 'url-test', 'load-balance', 'relay'];

interface GroupFormProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  group?: ProxyGroup;
  onSave: (data: {
    name: string;
    type: string;
    config: Record<string, any>;
    members: string[];
  }) => void;
  availableMembers: string[];
}

interface FormState {
  name: string;
  type: string;
  members: string[];
  // url-test / fallback
  url: string;
  interval: string;
  // load-balance
  strategy: string;
}

const DEFAULT_STATE: FormState = {
  name: '',
  type: 'select',
  members: [],
  url: 'http://www.gstatic.com/generate_204',
  interval: '300',
  strategy: 'consistent-hashing',
};

function parseMembers(raw: string[] | string | undefined): string[] {
  if (!raw) return [];
  if (Array.isArray(raw)) return raw;
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

export function GroupForm({ open, onOpenChange, group, onSave, availableMembers }: GroupFormProps) {
  const [form, setForm] = useState<FormState>(DEFAULT_STATE);

  useEffect(() => {
    if (open) {
      if (group) {
        setForm({
          name: group.name,
          type: group.type,
          members: parseMembers(group.members as any),
          url: group.config.url ? String(group.config.url) : 'http://www.gstatic.com/generate_204',
          interval: group.config.interval ? String(group.config.interval) : '300',
          strategy: group.config.strategy ? String(group.config.strategy) : 'consistent-hashing',
        });
      } else {
        setForm(DEFAULT_STATE);
      }
    }
  }, [open, group]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const config: Record<string, any> = {};

    if (form.type === 'url-test' || form.type === 'fallback') {
      config.url = form.url;
      config.interval = form.interval ? Number(form.interval) : 300;
    }

    if (form.type === 'load-balance') {
      config.strategy = form.strategy;
    }

    onSave({ name: form.name, type: form.type, config, members: form.members });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg overflow-y-auto max-h-[90vh]">
        <DialogHeader>
          <DialogTitle>{group ? 'Edit Proxy Group' : 'Add Proxy Group'}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="group-name">Name</Label>
            <Input
              id="group-name"
              type="text"
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              placeholder="Group name"
              required
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="group-type">Type</Label>
            <Select
              value={form.type}
              onValueChange={(v) => v && setForm((p) => ({ ...p, type: v }))}
            >
              <SelectTrigger className="w-full" id="group-type">
                <SelectValue placeholder="Select type" />
              </SelectTrigger>
              <SelectContent>
                {GROUP_TYPES.map((t) => (
                  <SelectItem key={t} value={t}>
                    {t}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {(form.type === 'url-test' || form.type === 'fallback') && (
            <>
              <div className="space-y-1.5">
                <Label htmlFor="group-url">Health Check URL</Label>
                <Input
                  id="group-url"
                  type="text"
                  value={form.url}
                  onChange={(e) => setForm((p) => ({ ...p, url: e.target.value }))}
                  placeholder="http://www.gstatic.com/generate_204"
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="group-interval">Interval (seconds)</Label>
                <Input
                  id="group-interval"
                  type="number"
                  value={form.interval}
                  onChange={(e) => setForm((p) => ({ ...p, interval: e.target.value }))}
                  placeholder="300"
                  min={10}
                />
              </div>
            </>
          )}

          {form.type === 'load-balance' && (
            <div className="space-y-1.5">
              <Label htmlFor="group-strategy">Strategy</Label>
              <Select
                value={form.strategy}
                onValueChange={(v) => v && setForm((p) => ({ ...p, strategy: v }))}
              >
                <SelectTrigger className="w-full" id="group-strategy">
                  <SelectValue placeholder="Select strategy" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="consistent-hashing">consistent-hashing</SelectItem>
                  <SelectItem value="round-robin">round-robin</SelectItem>
                </SelectContent>
              </Select>
            </div>
          )}

          <div className="space-y-1.5">
            <Label>Members</Label>
            <GroupMemberList
              members={form.members}
              onMembersChange={(members) => setForm((p) => ({ ...p, members }))}
              availableItems={availableMembers}
            />
          </div>

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
