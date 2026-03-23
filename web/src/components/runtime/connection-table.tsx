import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { XIcon } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { runtimeApi } from '@/api/runtime';

interface Connection {
  id: string;
  metadata: {
    host: string;
    network: string;
    type: string;
    destinationPort?: string;
  };
  chains: string[];
  rule: string;
  rulePayload?: string;
  download: number;
  upload: number;
  start: string;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function formatRelativeTime(isoString: string): string {
  const now = Date.now();
  const start = new Date(isoString).getTime();
  const diffMs = now - start;
  const diffSec = Math.floor(diffMs / 1000);
  if (diffSec < 60) return `${diffSec}s ago`;
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHr = Math.floor(diffMin / 60);
  return `${diffHr}h ago`;
}

export function ConnectionTable() {
  const [connections, setConnections] = useState<Connection[]>([]);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(true);

  const fetchConnections = useCallback(async () => {
    try {
      const res = await runtimeApi.connections();
      const data = res?.data;
      if (data && typeof data === 'object' && Array.isArray(data.connections)) {
        setConnections(data.connections);
      } else if (Array.isArray(data)) {
        setConnections(data);
      } else {
        setConnections([]);
      }
    } catch {
      // Silently ignore — mihomo may not be running
      setConnections([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchConnections();
    const interval = setInterval(fetchConnections, 3000);
    return () => clearInterval(interval);
  }, [fetchConnections]);

  const handleClose = async (id: string) => {
    try {
      await runtimeApi.closeConnection(id);
      setConnections((prev) => prev.filter((c) => c.id !== id));
    } catch {
      toast.error('Failed to close connection');
    }
  };

  const filtered = connections.filter((c) => {
    if (!search) return true;
    const term = search.toLowerCase();
    return (
      c.metadata.host?.toLowerCase().includes(term) ||
      c.rule?.toLowerCase().includes(term) ||
      c.chains?.some((ch) => ch.toLowerCase().includes(term))
    );
  });

  if (loading) {
    return <div className="text-center py-12 text-muted-foreground">Loading connections...</div>;
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <Input
          placeholder="Search by host, rule, chain..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="max-w-xs"
        />
        <span className="text-sm text-muted-foreground">{filtered.length} connections</span>
      </div>

      {filtered.length === 0 ? (
        <div className="text-center py-12 text-muted-foreground">No active connections</div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Host</TableHead>
              <TableHead>Network</TableHead>
              <TableHead>Type</TableHead>
              <TableHead>Chains</TableHead>
              <TableHead>Rule</TableHead>
              <TableHead>Download</TableHead>
              <TableHead>Upload</TableHead>
              <TableHead>Time</TableHead>
              <TableHead className="w-10"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.map((conn) => (
              <TableRow key={conn.id}>
                <TableCell className="font-mono text-xs max-w-[180px] truncate">
                  {conn.metadata.host}
                  {conn.metadata.destinationPort ? `:${conn.metadata.destinationPort}` : ''}
                </TableCell>
                <TableCell>
                  <Badge variant="outline" className="uppercase text-xs">
                    {conn.metadata.network}
                  </Badge>
                </TableCell>
                <TableCell className="text-xs">{conn.metadata.type}</TableCell>
                <TableCell className="max-w-[160px]">
                  <div className="flex flex-wrap gap-1">
                    {(conn.chains ?? []).map((ch, i) => (
                      <Badge key={i} variant="secondary" className="text-xs">
                        {ch}
                      </Badge>
                    ))}
                  </div>
                </TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {conn.rule}
                  {conn.rulePayload ? ` (${conn.rulePayload})` : ''}
                </TableCell>
                <TableCell className="text-xs">{formatBytes(conn.download)}</TableCell>
                <TableCell className="text-xs">{formatBytes(conn.upload)}</TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {formatRelativeTime(conn.start)}
                </TableCell>
                <TableCell>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => handleClose(conn.id)}
                    title="Close connection"
                  >
                    <XIcon className="h-3 w-3" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
