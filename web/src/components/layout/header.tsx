import { useNavigate } from 'react-router-dom';
import { LogOut, User, ChevronDown } from 'lucide-react';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useAuthStore } from '@/stores/auth';

export function Header() {
  const navigate = useNavigate();
  const { user, logout } = useAuthStore();

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  return (
    <header className="h-14 border-b flex items-center justify-end px-6 shrink-0 bg-background">
      <DropdownMenu>
        <DropdownMenuTrigger className="flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium hover:bg-accent transition-colors outline-none">
          <User className="h-4 w-4" />
          <span>{user?.username ?? 'Account'}</span>
          <ChevronDown className="h-3 w-3 text-muted-foreground" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-48">
          <DropdownMenuLabel>
            <div className="flex flex-col gap-0.5">
              <span className="font-medium text-sm text-foreground">{user?.username}</span>
              <span className="text-xs text-muted-foreground capitalize">{user?.role}</span>
            </div>
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onClick={handleLogout}
            className="gap-2 text-destructive focus:text-destructive"
          >
            <LogOut className="h-4 w-4" />
            Sign out
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </header>
  );
}
