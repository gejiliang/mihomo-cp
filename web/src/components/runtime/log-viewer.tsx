import { cn } from '@/lib/utils';
import { useT } from '@/i18n';

interface LogEntry {
  type: string;
  payload: string;
  time?: string;
}

const LEVEL_CLASSES: Record<string, string> = {
  error: 'text-red-500 dark:text-red-400',
  warning: 'text-yellow-600 dark:text-yellow-400',
  warn: 'text-yellow-600 dark:text-yellow-400',
  info: 'text-blue-600 dark:text-blue-400',
  debug: 'text-muted-foreground',
};

interface LogViewerProps {
  logs: LogEntry[];
}

export function LogViewer({ logs }: LogViewerProps) {
  const t = useT();
  if (logs.length === 0) {
    return (
      <div className="rounded-md bg-muted/50 p-4 text-center text-sm text-muted-foreground">
        {t('runtime.noLogs')}
      </div>
    );
  }

  return (
    <div className="rounded-md bg-muted/50 p-3 font-mono text-xs space-y-0.5 max-h-[480px] overflow-y-auto">
      {logs.map((entry, i) => {
        const level = entry.type?.toLowerCase() ?? 'info';
        const colorClass = LEVEL_CLASSES[level] ?? 'text-foreground';
        return (
          <div key={i} className="flex items-start gap-2">
            {entry.time && (
              <span className="text-muted-foreground shrink-0">{entry.time}</span>
            )}
            <span className={cn('uppercase font-semibold w-12 shrink-0', colorClass)}>
              {entry.type}
            </span>
            <span className="break-all">{entry.payload}</span>
          </div>
        );
      })}
    </div>
  );
}
