import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { CheckCircle2, Clock } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { publishApi, type PublishPreview, type PublishRecord } from '@/api/publish';
import { useDraftStore } from '@/stores/draft';
import { PublishPreview as PublishPreviewComponent } from '@/components/publish/publish-preview';
import { HistoryList } from '@/components/publish/history-list';
import { useT } from '@/i18n';

export default function PublishPage() {
  const t = useT();
  const { hasChanges, runningVersion, setStatus } = useDraftStore();
  const [preview, setPreview] = useState<PublishPreview | null>(null);
  const [history, setHistory] = useState<PublishRecord[]>([]);
  const [loadingStatus, setLoadingStatus] = useState(true);
  const [loadingPreview, setLoadingPreview] = useState(false);
  const [loadingHistory, setLoadingHistory] = useState(false);

  const fetchStatus = useCallback(async () => {
    try {
      const res = await publishApi.status();
      setStatus(res.data.has_changes, res.data.running_version);
    } catch {
      // silently ignore
    } finally {
      setLoadingStatus(false);
    }
  }, [setStatus]);

  const fetchPreview = useCallback(async () => {
    setLoadingPreview(true);
    try {
      const res = await publishApi.preview();
      setPreview(res.data);
    } catch {
      toast.error(t('publish.previewFailed'));
    } finally {
      setLoadingPreview(false);
    }
  }, []);

  const fetchHistory = useCallback(async () => {
    setLoadingHistory(true);
    try {
      const res = await publishApi.history(20);
      setHistory(res.data ?? []);
    } catch {
      toast.error(t('publish.historyFailed'));
    } finally {
      setLoadingHistory(false);
    }
  }, []);

  useEffect(() => {
    fetchStatus();
    fetchHistory();
  }, [fetchStatus, fetchHistory]);

  useEffect(() => {
    if (hasChanges) {
      fetchPreview();
    } else {
      setPreview(null);
    }
  }, [hasChanges, fetchPreview]);

  const handlePublished = () => {
    fetchStatus();
    fetchHistory();
    toast.success(t('publish.publishedSuccess'));
  };

  const handleRolledBack = () => {
    fetchStatus();
    fetchHistory();
  };

  const handleDiscarded = () => {
    fetchStatus();
    fetchPreview();
  };

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">{t('publish.title')}</h1>

      {/* Draft status card */}
      <Card>
        <CardContent className="py-4">
          {loadingStatus ? (
            <p className="text-sm text-muted-foreground">{t('publish.loadingStatus')}</p>
          ) : hasChanges ? (
            <div className="flex items-center gap-2 text-amber-600 dark:text-amber-400">
              <Clock className="h-4 w-4" />
              <span className="text-sm font-medium">{t('publish.draftPending')}</span>
            </div>
          ) : (
            <div className="flex items-center gap-2 text-green-600 dark:text-green-400">
              <CheckCircle2 className="h-4 w-4" />
              <span className="text-sm font-medium">
                {t('publish.upToDate')}
                {runningVersion > 0 && (
                  <span className="text-muted-foreground font-normal">
                    {' '}
                    — {t('publish.runningVersion', { version: String(runningVersion) })}
                  </span>
                )}
              </span>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Preview + publish controls (only when there are changes) */}
      {hasChanges && (
        <>
          {loadingPreview ? (
            <div className="text-center py-8 text-muted-foreground text-sm">
              {t('publish.loadingPreview')}
            </div>
          ) : preview ? (
            <PublishPreviewComponent preview={preview} onPublished={handlePublished} onDiscarded={handleDiscarded} />
          ) : null}
        </>
      )}

      {/* Publish history */}
      {loadingHistory ? (
        <div className="text-center py-8 text-muted-foreground text-sm">{t('publish.loadingHistory')}</div>
      ) : (
        <HistoryList records={history} onRolledBack={handleRolledBack} />
      )}
    </div>
  );
}
