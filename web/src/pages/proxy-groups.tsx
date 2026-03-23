import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { PlusIcon, PencilIcon, Trash2Icon } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { ConfirmDialog } from '@/components/shared/confirm-dialog';
import { GroupForm } from '@/components/proxy-groups/group-form';
import type { ProxyGroup } from '@/api/proxy-groups';
import { proxyGroupsApi } from '@/api/proxy-groups';
import { proxiesApi } from '@/api/proxies';

const GROUP_TYPES = ['select', 'fallback', 'url-test', 'load-balance', 'relay'];

const TYPE_VARIANT: Record<string, 'default' | 'secondary' | 'outline'> = {
  select: 'default',
  fallback: 'secondary',
  'url-test': 'default',
  'load-balance': 'secondary',
  relay: 'outline',
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

export default function ProxyGroupsPage() {
  const [groups, setGroups] = useState<ProxyGroup[]>([]);
  const [proxyNames, setProxyNames] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState('');
  const [typeFilter, setTypeFilter] = useState('');
  const [formOpen, setFormOpen] = useState(false);
  const [editingGroup, setEditingGroup] = useState<ProxyGroup | undefined>(undefined);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [confirmDescription, setConfirmDescription] = useState('');

  const fetchGroups = useCallback(async () => {
    setLoading(true);
    try {
      const params: { search?: string; type?: string } = {};
      if (search) params.search = search;
      if (typeFilter) params.type = typeFilter;
      const res = await proxyGroupsApi.list(params);
      setGroups(res.data ?? []);
    } catch {
      toast.error('Failed to load proxy groups');
    } finally {
      setLoading(false);
    }
  }, [search, typeFilter]);

  const fetchProxyNames = useCallback(async () => {
    try {
      const res = await proxiesApi.list();
      setProxyNames((res.data ?? []).map((p) => p.name));
    } catch {
      // non-fatal
    }
  }, []);

  useEffect(() => {
    fetchGroups();
  }, [fetchGroups]);

  useEffect(() => {
    fetchProxyNames();
  }, [fetchProxyNames]);

  const handleAddClick = () => {
    setEditingGroup(undefined);
    setFormOpen(true);
  };

  const handleEditClick = (group: ProxyGroup) => {
    setEditingGroup(group);
    setFormOpen(true);
  };

  const handleSave = async (data: {
    name: string;
    type: string;
    config: Record<string, any>;
    members: string[];
  }) => {
    try {
      if (editingGroup) {
        await proxyGroupsApi.update(editingGroup.id, data);
        toast.success('Proxy group updated');
      } else {
        await proxyGroupsApi.create(data);
        toast.success('Proxy group created');
      }
      setFormOpen(false);
      setEditingGroup(undefined);
      fetchGroups();
    } catch {
      toast.error(editingGroup ? 'Failed to update proxy group' : 'Failed to create proxy group');
    }
  };

  const handleDeleteClick = async (group: ProxyGroup) => {
    try {
      const res = await proxyGroupsApi.refs(group.id);
      const refs = res.data ?? [];
      if (refs.length > 0) {
        setConfirmDescription(
          `This group is referenced by ${refs.length} other group(s): ${refs.join(', ')}. Deleting it may break those groups. Continue?`
        );
      } else {
        setConfirmDescription(
          `Are you sure you want to delete "${group.name}"? This action cannot be undone.`
        );
      }
    } catch {
      setConfirmDescription(
        `Are you sure you want to delete "${group.name}"? This action cannot be undone.`
      );
    }
    setDeletingId(group.id);
    setConfirmOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!deletingId) return;
    try {
      await proxyGroupsApi.delete(deletingId);
      toast.success('Proxy group deleted');
      fetchGroups();
    } catch {
      toast.error('Failed to delete proxy group');
    } finally {
      setDeletingId(null);
    }
  };

  // Available members = all proxy names + all other group names (excluding the one being edited)
  const availableMembers = [
    ...proxyNames,
    ...groups
      .filter((g) => g.id !== editingGroup?.id)
      .map((g) => g.name),
  ];

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Proxy Groups</h1>
        <Button onClick={handleAddClick}>
          <PlusIcon />
          Add Group
        </Button>
      </div>

      <div className="flex items-center gap-2">
        <Input
          type="text"
          placeholder="Search groups..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="max-w-xs"
        />
        <Select
          value={typeFilter}
          onValueChange={(v) => setTypeFilter(!v || v === '__all__' ? '' : v)}
        >
          <SelectTrigger className="w-40">
            <SelectValue placeholder="All types" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">All types</SelectItem>
            {GROUP_TYPES.map((t) => (
              <SelectItem key={t} value={t}>
                {t}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {loading ? (
        <div className="text-center py-12 text-muted-foreground">Loading...</div>
      ) : groups.length === 0 ? (
        <div className="text-center py-12 text-muted-foreground">
          No proxy groups found. Click "Add Group" to create one.
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Type</TableHead>
              <TableHead>Members</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {groups.map((group) => {
              const members = parseMembers(group.members as any);
              const variant = TYPE_VARIANT[group.type] ?? 'outline';
              return (
                <TableRow key={group.id}>
                  <TableCell className="font-medium">{group.name}</TableCell>
                  <TableCell>
                    <Badge variant={variant}>{group.type}</Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground">{members.length}</TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => handleEditClick(group)}
                        title="Edit"
                      >
                        <PencilIcon />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => handleDeleteClick(group)}
                        title="Delete"
                      >
                        <Trash2Icon />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}

      <GroupForm
        open={formOpen}
        onOpenChange={(o) => {
          setFormOpen(o);
          if (!o) setEditingGroup(undefined);
        }}
        group={editingGroup}
        onSave={handleSave}
        availableMembers={availableMembers}
      />

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Delete Proxy Group"
        description={confirmDescription}
        onConfirm={handleDeleteConfirm}
        variant="destructive"
      />
    </div>
  );
}
