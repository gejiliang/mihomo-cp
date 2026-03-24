import { useState } from 'react';
import { toast } from 'sonner';
import { ChevronDown, ChevronUp, RotateCcw } from 'lucide-react';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { ConfirmDialog } from '@/components/shared/confirm-dialog';
import { publishApi, type PublishRecord } from '@/api/publish';
import { useT } from '@/i18n';

function StatusBadge({ status }: { status: string }) {
  if (status === 'success') {
    return (
      <Badge className="bg-green-500/10 text-green-700 dark:text-green-400 hover:bg-green-500/20 border border-green-500/30">
        Success
      </Badge>
    );
  }
  if (status === 'failed') {
    return <Badge variant="destructive">Failed</Badge>;
  }
  if (status === 'rolled_back') {
    return (
      <Badge className="bg-yellow-500/10 text-yellow-700 dark:text-yellow-400 hover:bg-yellow-500/20 border border-yellow-500/30">
        Rolled back
      </Badge>
    );
  }
  return <Badge variant="secondary">{status}</Badge>;
}

function formatTime(ts: string): string {
  if (!ts) return '—';
  try {
    return new Date(ts).toLocaleString();
  } catch {
    return ts;
  }
}

interface HistoryListProps {
  records: PublishRecord[];
  onRolledBack: () => void;
}

export function HistoryList({ records, onRolledBack }: HistoryListProps) {
  const t = useT();
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [rollbackDialogOpen, setRollbackDialogOpen] = useState(false);
  const [rollingBack, setRollingBack] = useState(false);

  const toggleExpand = (id: string) => {
    setExpandedId((prev) => (prev === id ? null : id));
  };

  const handleRollback = async () => {
    setRollingBack(true);
    try {
      await publishApi.rollback();
      toast.success(t('publish.rollbackSuccess'));
      onRolledBack();
    } catch {
      toast.error(t('publish.rollbackFailed'));
    } finally {
      setRollingBack(false);
    }
  };

  if (records.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('publish.history')}</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground text-center py-8">{t('publish.noHistory')}</p>
        </CardContent>
      </Card>
    );
  }

  // The latest successful record is the currently running one; only show rollback on older successful records
  const latestSuccessIdx = records.findIndex((r) => r.status === 'success');

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('publish.history')}</CardTitle>
      </CardHeader>
      <CardContent className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-16">{t('overview.version')}</TableHead>
              <TableHead>{t('overview.time')}</TableHead>
              <TableHead>{t('overview.operator')}</TableHead>
              <TableHead>{t('overview.status')}</TableHead>
              <TableHead className="max-w-xs">{t('overview.note')}</TableHead>
              <TableHead className="text-right">{t('common.actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {records.map((record, idx) => (
              <>
                <TableRow key={record.id}>
                  <TableCell className="font-mono text-sm">v{record.version}</TableCell>
                  <TableCell className="text-sm text-muted-foreground whitespace-nowrap">
                    {formatTime(record.created_at)}
                  </TableCell>
                  <TableCell className="text-sm">{record.operator || '—'}</TableCell>
                  <TableCell>
                    <StatusBadge status={record.status} />
                  </TableCell>
                  <TableCell className="max-w-xs text-sm text-muted-foreground truncate">
                    {record.note || '—'}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-2">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => toggleExpand(record.id)}
                        className="h-7 px-2"
                      >
                        {expandedId === record.id ? (
                          <ChevronUp className="h-4 w-4" />
                        ) : (
                          <ChevronDown className="h-4 w-4" />
                        )}
                        {t('common.view')}
                      </Button>
                      {record.status === 'success' && idx !== latestSuccessIdx && (
                        <Button
                          variant="outline"
                          size="sm"
                          className="h-7 px-2 text-destructive hover:text-destructive"
                          onClick={() => setRollbackDialogOpen(true)}
                          disabled={rollingBack}
                        >
                          <RotateCcw className="h-3 w-3 mr-1" />
                          {t('publish.rollback')}
                        </Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
                {expandedId === record.id && (
                  <TableRow key={`${record.id}-expand`}>
                    <TableCell colSpan={6} className="bg-muted/30 p-4">
                      <div className="space-y-3">
                        {record.error_msg && (
                          <div>
                            <p className="text-xs font-medium text-destructive mb-1">{t('publish.error')}</p>
                            <pre className="text-xs font-mono text-destructive/80">
                              {record.error_msg}
                            </pre>
                          </div>
                        )}
                        {record.diff_text && (
                          <div>
                            <p className="text-xs font-medium text-muted-foreground mb-1">{t('publish.diff')}</p>
                            <div className="rounded border bg-background p-3 max-h-48 overflow-auto">
                              <pre className="text-xs font-mono whitespace-pre-wrap">
                                {record.diff_text.split('\n').map((line, i) => {
                                  let cls = '';
                                  if (line.startsWith('+'))
                                    cls = 'text-green-700 dark:text-green-400';
                                  else if (line.startsWith('-'))
                                    cls = 'text-red-700 dark:text-red-400';
                                  return (
                                    <div key={i} className={cls}>
                                      {line || ' '}
                                    </div>
                                  );
                                })}
                              </pre>
                            </div>
                          </div>
                        )}
                        {record.config_yaml && (
                          <div>
                            <p className="text-xs font-medium text-muted-foreground mb-1">
                              {t('publish.config')}
                            </p>
                            <div className="rounded border bg-background p-3 max-h-64 overflow-auto">
                              <pre className="text-xs font-mono whitespace-pre-wrap">
                                {record.config_yaml}
                              </pre>
                            </div>
                          </div>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                )}
              </>
            ))}
          </TableBody>
        </Table>
      </CardContent>

      <ConfirmDialog
        open={rollbackDialogOpen}
        onOpenChange={setRollbackDialogOpen}
        title={t('publish.rollbackTitle')}
        description={t('publish.rollbackConfirm')}
        onConfirm={handleRollback}
        variant="destructive"
      />
    </Card>
  );
}
