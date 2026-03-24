import { useEffect, useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { LogOut, User, ChevronDown, KeyRound, Languages } from 'lucide-react';
import { toast } from 'sonner';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { useAuthStore } from '@/stores/auth';
import { useDraftStore } from '@/stores/draft';
import { publishApi } from '@/api/publish';
import { authApi } from '@/api/auth';
import { useT, useI18nStore } from '@/i18n';

export function Header() {
  const navigate = useNavigate();
  const t = useT();
  const { locale, setLocale } = useI18nStore();
  const { user, logout } = useAuthStore();
  const { hasChanges, setStatus } = useDraftStore();
  const [pwDialogOpen, setPwDialogOpen] = useState(false);
  const [currentPw, setCurrentPw] = useState('');
  const [newPw, setNewPw] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    const fetchStatus = async () => {
      try {
        const res = await publishApi.status();
        setStatus(res.data.has_changes, res.data.running_version);
      } catch {
        // silently ignore
      }
    };

    fetchStatus();
    const interval = setInterval(fetchStatus, 30_000);
    return () => clearInterval(interval);
  }, [setStatus]);

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  const handleChangePassword = async () => {
    if (!currentPw || !newPw) {
      toast.error(t('header.fillBothFields'));
      return;
    }
    setSaving(true);
    try {
      await authApi.changePassword(currentPw, newPw);
      toast.success(t('header.passwordChanged'));
      setPwDialogOpen(false);
      setCurrentPw('');
      setNewPw('');
    } catch {
      toast.error(t('header.passwordChangeFailed'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <header className="h-14 border-b flex items-center justify-between px-6 shrink-0 bg-background">
        <div className="flex items-center gap-3">
          {hasChanges && (
            <Link to="/publish">
              <Badge className="bg-amber-500/10 text-amber-700 dark:text-amber-400 hover:bg-amber-500/20 border border-amber-500/30 cursor-pointer">
                {t('header.changesPending')}
              </Badge>
            </Link>
          )}
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger className="flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium hover:bg-accent transition-colors outline-none">
            <User className="h-4 w-4" />
            <span>{user?.username ?? t('header.account')}</span>
            <ChevronDown className="h-3 w-3 text-muted-foreground" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            <div className="px-1.5 py-1.5">
              <div className="font-medium text-sm text-foreground">{user?.username}</div>
              <div className="text-xs text-muted-foreground capitalize">{user?.role}</div>
            </div>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={() => setPwDialogOpen(true)}
              className="gap-2"
            >
              <KeyRound className="h-4 w-4" />
              {t('header.changePassword')}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => setLocale(locale === 'en' ? 'zh' : 'en')}
              className="gap-2"
            >
              <Languages className="h-4 w-4" />
              {t('header.language')}: {locale === 'en' ? 'English' : '中文'}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={handleLogout}
              className="gap-2 text-destructive focus:text-destructive"
            >
              <LogOut className="h-4 w-4" />
              {t('header.signOut')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </header>

      <Dialog open={pwDialogOpen} onOpenChange={(o) => {
        setPwDialogOpen(o);
        if (!o) { setCurrentPw(''); setNewPw(''); }
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('header.changePassword')}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="current-password">{t('header.currentPassword')}</Label>
              <Input
                id="current-password"
                type="password"
                value={currentPw}
                onChange={(e) => setCurrentPw(e.target.value)}
                placeholder={t('header.currentPasswordPlaceholder')}
                autoComplete="current-password"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="new-password">{t('header.newPassword')}</Label>
              <Input
                id="new-password"
                type="password"
                value={newPw}
                onChange={(e) => setNewPw(e.target.value)}
                placeholder={t('header.newPasswordPlaceholder')}
                autoComplete="new-password"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPwDialogOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button onClick={handleChangePassword} disabled={saving}>
              {saving ? t('common.saving') : t('header.changePassword')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
