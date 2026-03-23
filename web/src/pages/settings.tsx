import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { PlusIcon, PencilIcon, Trash2Icon } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { ConfirmDialog } from '@/components/shared/confirm-dialog';
import { useAuthStore } from '@/stores/auth';
import { settingsApi } from '@/api/settings';

// ─── App Settings ─────────────────────────────────────────────────────────────

interface AppSettings {
  mihomo_config_path?: string;
  mihomo_working_dir?: string;
  mihomo_binary_path?: string;
  external_controller_url?: string;
  external_controller_secret?: string;
}

function AppSettingsCard() {
  const [settings, setSettings] = useState<AppSettings>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const fetchSettings = useCallback(async () => {
    try {
      const res = await settingsApi.get();
      setSettings(res.data ?? {});
    } catch {
      // Silently ignore
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchSettings();
  }, [fetchSettings]);

  const handleChange = (key: keyof AppSettings, value: string) => {
    setSettings((prev) => ({ ...prev, [key]: value }));
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      await settingsApi.update(settings);
      toast.success('Settings saved');
    } catch {
      toast.error('Failed to save settings');
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-muted-foreground text-sm">
          Loading settings...
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader className="border-b pb-3">
        <CardTitle>App Settings</CardTitle>
      </CardHeader>
      <CardContent className="pt-4 space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="config-path">mihomo Config Path</Label>
            <Input
              id="config-path"
              value={settings.mihomo_config_path ?? ''}
              onChange={(e) => handleChange('mihomo_config_path', e.target.value)}
              placeholder="/etc/mihomo/config.yaml"
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="working-dir">mihomo Working Directory</Label>
            <Input
              id="working-dir"
              value={settings.mihomo_working_dir ?? ''}
              onChange={(e) => handleChange('mihomo_working_dir', e.target.value)}
              placeholder="/etc/mihomo"
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="binary-path">mihomo Binary Path</Label>
            <Input
              id="binary-path"
              value={settings.mihomo_binary_path ?? ''}
              onChange={(e) => handleChange('mihomo_binary_path', e.target.value)}
              placeholder="/usr/local/bin/mihomo"
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="controller-url">External Controller URL</Label>
            <Input
              id="controller-url"
              value={settings.external_controller_url ?? ''}
              onChange={(e) => handleChange('external_controller_url', e.target.value)}
              placeholder="http://127.0.0.1:9090"
            />
          </div>
          <div className="space-y-1.5 sm:col-span-2">
            <Label htmlFor="controller-secret">External Controller Secret</Label>
            <Input
              id="controller-secret"
              type="password"
              value={settings.external_controller_secret ?? ''}
              onChange={(e) => handleChange('external_controller_secret', e.target.value)}
              placeholder="Leave blank if no secret"
            />
          </div>
        </div>
        <div className="flex justify-end">
          <Button onClick={handleSave} disabled={saving}>
            {saving ? 'Saving...' : 'Save Settings'}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

// ─── User Management ──────────────────────────────────────────────────────────

interface User {
  id: string;
  username: string;
  role: string;
}

interface UserDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  editingUser?: User;
  onSaved: () => void;
}

function UserDialog({ open, onOpenChange, editingUser, onSaved }: UserDialogProps) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [role, setRole] = useState('readonly');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (open) {
      setUsername(editingUser?.username ?? '');
      setPassword('');
      setRole(editingUser?.role ?? 'readonly');
    }
  }, [open, editingUser]);

  const handleSave = async () => {
    if (!editingUser && !username.trim()) {
      toast.error('Username is required');
      return;
    }
    if (!editingUser && !password.trim()) {
      toast.error('Password is required');
      return;
    }
    setSaving(true);
    try {
      if (editingUser) {
        const data: any = { role };
        if (password.trim()) data.password = password;
        await settingsApi.updateUser(editingUser.id, data);
        toast.success('User updated');
      } else {
        await settingsApi.createUser({ username: username.trim(), password, role });
        toast.success('User created');
      }
      onSaved();
      onOpenChange(false);
    } catch {
      toast.error(editingUser ? 'Failed to update user' : 'Failed to create user');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{editingUser ? 'Edit User' : 'Add User'}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          {!editingUser && (
            <div className="space-y-1.5">
              <Label htmlFor="new-username">Username</Label>
              <Input
                id="new-username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="username"
                autoComplete="off"
              />
            </div>
          )}
          <div className="space-y-1.5">
            <Label htmlFor="new-password">
              Password{editingUser ? ' (leave blank to keep current)' : ''}
            </Label>
            <Input
              id="new-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={editingUser ? 'New password (optional)' : 'Password'}
              autoComplete="new-password"
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="new-role">Role</Label>
            <Select value={role} onValueChange={(v) => { if (v) setRole(v); }}>
              <SelectTrigger id="new-role">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="admin">Admin</SelectItem>
                <SelectItem value="readonly">Readonly</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleSave} disabled={saving}>
            {saving ? 'Saving...' : editingUser ? 'Update' : 'Create'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function UserManagementCard() {
  const currentUser = useAuthStore((s) => s.user);
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingUser, setEditingUser] = useState<User | undefined>(undefined);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);

  const fetchUsers = useCallback(async () => {
    try {
      const res = await settingsApi.listUsers();
      setUsers(res.data ?? []);
    } catch {
      setUsers([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchUsers();
  }, [fetchUsers]);

  const handleAddClick = () => {
    setEditingUser(undefined);
    setDialogOpen(true);
  };

  const handleEditClick = (user: User) => {
    setEditingUser(user);
    setDialogOpen(true);
  };

  const handleDeleteClick = (user: User) => {
    if (user.id === currentUser?.id) {
      toast.error('You cannot delete your own account');
      return;
    }
    setDeletingId(user.id);
    setConfirmOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!deletingId) return;
    try {
      await settingsApi.deleteUser(deletingId);
      toast.success('User deleted');
      fetchUsers();
    } catch {
      toast.error('Failed to delete user');
    } finally {
      setDeletingId(null);
    }
  };

  const isAdmin = currentUser?.role === 'admin';
  if (!isAdmin) return null;

  return (
    <Card>
      <CardHeader className="border-b pb-3">
        <div className="flex items-center justify-between">
          <CardTitle>User Management</CardTitle>
          <Button size="sm" onClick={handleAddClick}>
            <PlusIcon className="h-4 w-4 mr-1.5" />
            Add User
          </Button>
        </div>
      </CardHeader>
      <CardContent className="pt-0">
        {loading ? (
          <p className="text-center py-8 text-sm text-muted-foreground">Loading users...</p>
        ) : users.length === 0 ? (
          <p className="text-center py-8 text-sm text-muted-foreground">No users found.</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Username</TableHead>
                <TableHead>Role</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.map((user) => (
                <TableRow key={user.id}>
                  <TableCell className="font-medium">
                    {user.username}
                    {user.id === currentUser?.id && (
                      <span className="ml-2 text-xs text-muted-foreground">(you)</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <Badge variant={user.role === 'admin' ? 'default' : 'secondary'}>
                      {user.role}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => handleEditClick(user)}
                        title="Edit"
                      >
                        <PencilIcon />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => handleDeleteClick(user)}
                        title="Delete"
                        disabled={user.id === currentUser?.id}
                      >
                        <Trash2Icon />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>

      <UserDialog
        open={dialogOpen}
        onOpenChange={(o) => {
          setDialogOpen(o);
          if (!o) setEditingUser(undefined);
        }}
        editingUser={editingUser}
        onSaved={fetchUsers}
      />

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Delete User"
        description="Are you sure you want to delete this user? This action cannot be undone."
        onConfirm={handleDeleteConfirm}
        variant="destructive"
      />
    </Card>
  );
}

// ─── Settings Page ────────────────────────────────────────────────────────────

export default function SettingsPage() {
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Settings</h1>
      <AppSettingsCard />
      <UserManagementCard />
    </div>
  );
}
