import { useState } from 'react';
import { toast } from 'sonner';
import { CheckCircle2, XCircle, AlertCircle } from 'lucide-react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { publishApi, type PublishPreview, type ValidationResult } from '@/api/publish';

function DiffView({ diff }: { diff: string }) {
  const lines = diff.split('\n');
  return (
    <pre className="text-sm font-mono whitespace-pre-wrap">
      {lines.map((line, i) => {
        let className = '';
        if (line.startsWith('+')) className = 'bg-green-500/10 text-green-700 dark:text-green-400';
        else if (line.startsWith('-'))
          className = 'bg-red-500/10 text-red-700 dark:text-red-400';
        return (
          <div key={i} className={className}>
            {line || ' '}
          </div>
        );
      })}
    </pre>
  );
}

interface PublishPreviewProps {
  preview: PublishPreview;
  onPublished: () => void;
}

export function PublishPreview({ preview, onPublished }: PublishPreviewProps) {
  const [validating, setValidating] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const [validation, setValidation] = useState<ValidationResult | null>(null);
  const [note, setNote] = useState('');

  const handleValidate = async () => {
    setValidating(true);
    try {
      const res = await publishApi.validate();
      setValidation(res.data);
      if (res.data.valid) {
        toast.success('Configuration is valid');
      } else {
        toast.error(`Validation failed: ${res.data.errors.length} error(s)`);
      }
    } catch {
      toast.error('Validation failed');
    } finally {
      setValidating(false);
    }
  };

  const handlePublish = async () => {
    setPublishing(true);
    try {
      await publishApi.publish(note || undefined);
      toast.success('Published successfully');
      setNote('');
      setValidation(null);
      onPublished();
    } catch {
      toast.error('Publish failed');
    } finally {
      setPublishing(false);
    }
  };

  const canPublish = validation?.valid === true;

  const validationBadge = () => {
    if (!validation) {
      return <Badge variant="secondary">Not validated</Badge>;
    }
    if (validation.valid) {
      return (
        <Badge className="bg-green-500/10 text-green-700 dark:text-green-400 hover:bg-green-500/20 border border-green-500/30">
          <CheckCircle2 className="h-3 w-3 mr-1" />
          Valid
        </Badge>
      );
    }
    return (
      <Badge variant="destructive">
        <XCircle className="h-3 w-3 mr-1" />
        Invalid ({validation.errors.length} error{validation.errors.length !== 1 ? 's' : ''})
      </Badge>
    );
  };

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <CardTitle>Preview & Publish</CardTitle>
          {validationBadge()}
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <Tabs defaultValue="diff">
          <TabsList>
            <TabsTrigger value="diff">Diff</TabsTrigger>
            <TabsTrigger value="preview">Full YAML</TabsTrigger>
          </TabsList>
          <TabsContent value="diff">
            <div className="rounded-md border bg-muted/30 p-4 max-h-96 overflow-auto">
              {preview.diff ? (
                <DiffView diff={preview.diff} />
              ) : (
                <p className="text-sm text-muted-foreground">No differences</p>
              )}
            </div>
          </TabsContent>
          <TabsContent value="preview">
            <div className="rounded-md border bg-muted/30 p-4 max-h-96 overflow-auto">
              <pre className="text-sm font-mono whitespace-pre-wrap">{preview.yaml}</pre>
            </div>
          </TabsContent>
        </Tabs>

        {validation && validation.errors.length > 0 && (
          <div className="space-y-2">
            {validation.errors.map((err, i) => (
              <Alert
                key={i}
                variant={err.level === 'error' ? 'destructive' : 'default'}
                className="py-2"
              >
                <AlertCircle className="h-4 w-4" />
                <AlertDescription className="text-sm">
                  <span className="font-mono text-xs mr-2 text-muted-foreground">[{err.code}]</span>
                  {err.message}
                </AlertDescription>
              </Alert>
            ))}
          </div>
        )}

        {validation?.output && (
          <div className="rounded-md border bg-muted/30 p-3">
            <p className="text-xs text-muted-foreground mb-1 font-medium">Validation output</p>
            <pre className="text-xs font-mono whitespace-pre-wrap">{validation.output}</pre>
          </div>
        )}

        <Textarea
          placeholder="Release note (optional)"
          value={note}
          onChange={(e) => setNote(e.target.value)}
          rows={2}
        />

        <div className="flex items-center gap-3">
          <Button variant="outline" onClick={handleValidate} disabled={validating}>
            {validating ? 'Validating...' : 'Validate'}
          </Button>
          <Button onClick={handlePublish} disabled={!canPublish || publishing}>
            {publishing ? 'Publishing...' : 'Publish'}
          </Button>
          {!canPublish && validation === null && (
            <p className="text-sm text-muted-foreground">Validate before publishing</p>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
