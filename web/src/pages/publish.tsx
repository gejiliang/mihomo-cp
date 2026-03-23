import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { CheckCircle2, Clock } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { publishApi, type PublishPreview, type PublishRecord } from '@/api/publish';
import { useDraftStore } from '@/stores/draft';
import { PublishPreview as PublishPreviewComponent } from '@/components/publish/publish-preview';
import { HistoryList } from '@/components/publish/history-list';

export default function PublishPage() {
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
      toast.error('Failed to load preview');
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
      toast.error('Failed to load history');
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
    toast.success('Configuration published successfully');
  };

  const handleRolledBack = () => {
    fetchStatus();
    fetchHistory();
  };

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Publish Center</h1>

      {/* Draft status card */}
      <Card>
        <CardContent className="py-4">
          {loadingStatus ? (
            <p className="text-sm text-muted-foreground">Loading status...</p>
          ) : hasChanges ? (
            <div className="flex items-center gap-2 text-amber-600 dark:text-amber-400">
              <Clock className="h-4 w-4" />
              <span className="text-sm font-medium">Draft changes pending — not yet published</span>
            </div>
          ) : (
            <div className="flex items-center gap-2 text-green-600 dark:text-green-400">
              <CheckCircle2 className="h-4 w-4" />
              <span className="text-sm font-medium">
                Up to date
                {runningVersion > 0 && (
                  <span className="text-muted-foreground font-normal">
                    {' '}
                    — running version v{runningVersion}
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
              Loading preview...
            </div>
          ) : preview ? (
            <PublishPreviewComponent preview={preview} onPublished={handlePublished} />
          ) : null}
        </>
      )}

      {/* Publish history */}
      {loadingHistory ? (
        <div className="text-center py-8 text-muted-foreground text-sm">Loading history...</div>
      ) : (
        <HistoryList records={history} onRolledBack={handleRolledBack} />
      )}
    </div>
  );
}
