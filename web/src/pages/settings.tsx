import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { PlusIcon, PencilIcon, Trash2Icon, FileTextIcon } from 'lucide-react';
import { useT } from '@/i18n';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Textarea } from '@/components/ui/textarea';
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
  const t = useT();
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
      toast.success(t('settings.saved'));
    } catch {
      toast.error(t('settings.saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-muted-foreground text-sm">
          {t('settings.loadingSettings')}
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader className="border-b pb-3">
        <CardTitle>{t('settings.appSettings')}</CardTitle>
      </CardHeader>
      <CardContent className="pt-4 space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="config-path">{t('settings.configPath')}</Label>
            <Input
              id="config-path"
              value={settings.mihomo_config_path ?? ''}
              onChange={(e) => handleChange('mihomo_config_path', e.target.value)}
              placeholder={t('settings.configPathPlaceholder')}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="working-dir">{t('settings.workingDir')}</Label>
            <Input
              id="working-dir"
              value={settings.mihomo_working_dir ?? ''}
              onChange={(e) => handleChange('mihomo_working_dir', e.target.value)}
              placeholder={t('settings.workingDirPlaceholder')}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="binary-path">{t('settings.binaryPath')}</Label>
            <Input
              id="binary-path"
              value={settings.mihomo_binary_path ?? ''}
              onChange={(e) => handleChange('mihomo_binary_path', e.target.value)}
              placeholder={t('settings.binaryPathPlaceholder')}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="controller-url">{t('settings.controllerUrl')}</Label>
            <Input
              id="controller-url"
              value={settings.external_controller_url ?? ''}
              onChange={(e) => handleChange('external_controller_url', e.target.value)}
              placeholder={t('settings.controllerUrlPlaceholder')}
            />
          </div>
          <div className="space-y-1.5 sm:col-span-2">
            <Label htmlFor="controller-secret">{t('settings.controllerSecret')}</Label>
            <Input
              id="controller-secret"
              type="password"
              value={settings.external_controller_secret ?? ''}
              onChange={(e) => handleChange('external_controller_secret', e.target.value)}
              placeholder={t('settings.controllerSecretPlaceholder')}
            />
          </div>
        </div>
        <div className="flex justify-end">
          <Button onClick={handleSave} disabled={saving}>
            {saving ? t('common.saving') : t('settings.saveSettings')}
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
  const t = useT();
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
      toast.error(t('settings.usernameRequired'));
      return;
    }
    if (!editingUser && !password.trim()) {
      toast.error(t('settings.passwordRequired'));
      return;
    }
    setSaving(true);
    try {
      if (editingUser) {
        const data: any = { role };
        if (password.trim()) data.password = password;
        await settingsApi.updateUser(editingUser.id, data);
        toast.success(t('settings.userUpdated'));
      } else {
        await settingsApi.createUser({ username: username.trim(), password, role });
        toast.success(t('settings.userCreated'));
      }
      onSaved();
      onOpenChange(false);
    } catch {
      toast.error(editingUser ? t('settings.userUpdateFailed') : t('settings.userCreateFailed'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{editingUser ? t('settings.editUserDialog') : t('settings.addUserDialog')}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          {!editingUser && (
            <div className="space-y-1.5">
              <Label htmlFor="new-username">{t('settings.username')}</Label>
              <Input
                id="new-username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder={t('settings.usernamePlaceholder')}
                autoComplete="off"
              />
            </div>
          )}
          <div className="space-y-1.5">
            <Label htmlFor="new-password">
              {editingUser ? t('settings.passwordEditLabel') : t('settings.passwordLabel')}
            </Label>
            <Input
              id="new-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={editingUser ? t('settings.passwordEditPlaceholder') : t('settings.passwordPlaceholder')}
              autoComplete="new-password"
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="new-role">{t('settings.role')}</Label>
            <Select value={role} onValueChange={(v) => { if (v) setRole(v); }}>
              <SelectTrigger id="new-role">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="admin">{t('settings.roleAdmin')}</SelectItem>
                <SelectItem value="readonly">{t('settings.roleReadonly')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleSave} disabled={saving}>
            {saving ? t('common.saving') : editingUser ? t('common.update') : t('common.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function UserManagementCard() {
  const t = useT();
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
      toast.error(t('settings.cannotDeleteSelf'));
      return;
    }
    setDeletingId(user.id);
    setConfirmOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!deletingId) return;
    try {
      await settingsApi.deleteUser(deletingId);
      toast.success(t('settings.userDeleted'));
      fetchUsers();
    } catch {
      toast.error(t('settings.userDeleteFailed'));
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
          <CardTitle>{t('settings.userManagement')}</CardTitle>
          <Button size="sm" onClick={handleAddClick}>
            <PlusIcon className="h-4 w-4 mr-1.5" />
            {t('settings.addUser')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="pt-0">
        {loading ? (
          <p className="text-center py-8 text-sm text-muted-foreground">{t('settings.loadingUsers')}</p>
        ) : users.length === 0 ? (
          <p className="text-center py-8 text-sm text-muted-foreground">{t('settings.noUsers')}</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('settings.username')}</TableHead>
                <TableHead>{t('settings.role')}</TableHead>
                <TableHead className="text-right">{t('common.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.map((user) => (
                <TableRow key={user.id}>
                  <TableCell className="font-medium">
                    {user.username}
                    {user.id === currentUser?.id && (
                      <span className="ml-2 text-xs text-muted-foreground">{t('settings.you')}</span>
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
                        title={t('common.edit')}
                      >
                        <PencilIcon />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => handleDeleteClick(user)}
                        title={t('common.delete')}
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
        title={t('settings.deleteUserTitle')}
        description={t('settings.deleteUserConfirm')}
        onConfirm={handleDeleteConfirm}
        variant="destructive"
      />
    </Card>
  );
}

// ─── Config Editor ────────────────────────────────────────────────────────────

function ConfigEditorCard() {
  const t = useT();
  const [content, setContent] = useState('');
  const [source, setSource] = useState<'file' | 'draft'>('file');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [clearing, setClearing] = useState(false);

  const fetchConfig = useCallback(async () => {
    try {
      const res = await settingsApi.getConfigYaml();
      setContent(res.data.content ?? '');
      setSource(res.data.source as 'file' | 'draft');
    } catch {
      toast.error(t('settings.configLoadFailed'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchConfig();
  }, [fetchConfig]);

  const handleSaveDraft = async () => {
    setSaving(true);
    try {
      await settingsApi.updateConfigYaml(content);
      setSource('draft');
      toast.success(t('settings.configSaved'));
    } catch {
      toast.error(t('settings.configSaveFailed'));
    } finally {
      setSaving(false);
    }
  };

  const handleClearDraft = async () => {
    setClearing(true);
    try {
      await settingsApi.deleteConfigYaml();
      toast.success(t('settings.configDraftCleared'));
      // Reload from file
      await fetchConfig();
    } catch {
      toast.error(t('settings.configClearFailed'));
    } finally {
      setClearing(false);
    }
  };

  return (
    <Card>
      <CardHeader className="border-b pb-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <FileTextIcon className="h-5 w-5 text-muted-foreground" />
            <CardTitle>{t('settings.configEditor')}</CardTitle>
          </div>
          <Badge variant={source === 'draft' ? 'default' : 'secondary'}>
            {source === 'draft' ? t('settings.draftIndicator') : t('settings.fileIndicator')}
          </Badge>
        </div>
        <p className="text-sm text-muted-foreground mt-1">{t('settings.configEditorDesc')}</p>
      </CardHeader>
      <CardContent className="pt-4 space-y-4">
        {loading ? (
          <p className="text-center py-8 text-sm text-muted-foreground">{t('settings.configLoading')}</p>
        ) : (
          <>
            <Textarea
              value={content}
              onChange={(e) => setContent(e.target.value)}
              className="font-mono text-sm min-h-[400px] resize-y"
              spellCheck={false}
            />
            <div className="flex items-center gap-3">
              <Button onClick={handleSaveDraft} disabled={saving}>
                {saving ? t('common.saving') : t('settings.saveDraft')}
              </Button>
              {source === 'draft' && (
                <Button variant="outline" onClick={handleClearDraft} disabled={clearing}>
                  {t('settings.clearDraft')}
                </Button>
              )}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}

// ─── Settings Page ────────────────────────────────────────────────────────────

export default function SettingsPage() {
  const t = useT();
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">{t('settings.title')}</h1>
      <AppSettingsCard />
      <ConfigEditorCard />
      <UserManagementCard />
    </div>
  );
}
