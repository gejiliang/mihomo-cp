import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { PlusIcon, AlertTriangleIcon } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { RuleList } from '@/components/rules/rule-list';
import { RuleForm } from '@/components/rules/rule-form';
import type { Rule } from '@/api/rules';
import { rulesApi } from '@/api/rules';
import { useT } from '@/i18n';

const RULE_TYPES = [
  'DOMAIN',
  'DOMAIN-SUFFIX',
  'DOMAIN-KEYWORD',
  'IP-CIDR',
  'IP-CIDR6',
  'SRC-IP-CIDR',
  'DST-PORT',
  'PROCESS-NAME',
  'GEOIP',
  'RULE-SET',
  'MATCH',
];

export default function RulesPage() {
  const t = useT();
  const [rules, setRules] = useState<Rule[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState('');
  const [typeFilter, setTypeFilter] = useState('');
  const [formOpen, setFormOpen] = useState(false);
  const [editingRule, setEditingRule] = useState<Rule | undefined>(undefined);

  const fetchRules = useCallback(async () => {
    setLoading(true);
    try {
      const params: { search?: string; type?: string } = {};
      if (search) params.search = search;
      if (typeFilter) params.type = typeFilter;
      const res = await rulesApi.list(params);
      setRules(res.data ?? []);
    } catch {
      toast.error(t('rules.loadFailed'));
    } finally {
      setLoading(false);
    }
  }, [search, typeFilter]);

  useEffect(() => {
    fetchRules();
  }, [fetchRules]);

  const handleAddClick = () => {
    setEditingRule(undefined);
    setFormOpen(true);
  };

  const handleEditClick = (rule: Rule) => {
    setEditingRule(rule);
    setFormOpen(true);
  };

  const handleSave = async (data: Partial<Rule>) => {
    try {
      if (editingRule) {
        await rulesApi.update(editingRule.id, data);
        toast.success(t('rules.updated'));
      } else {
        await rulesApi.create(data);
        toast.success(t('rules.created'));
      }
      setFormOpen(false);
      setEditingRule(undefined);
      fetchRules();
    } catch {
      toast.error(editingRule ? t('rules.updateFailed') : t('rules.createFailed'));
    }
  };

  const handleReorder = async (reorderedRules: Rule[]) => {
    setRules(reorderedRules);
    try {
      await rulesApi.reorder(reorderedRules.map((r) => r.id));
    } catch {
      toast.error(t('rules.orderSaveFailed'));
      fetchRules();
    }
  };

  const hasMatchRule = rules.some((r) => r.type === 'MATCH');

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t('rules.title')}</h1>
        <Button onClick={handleAddClick}>
          <PlusIcon />
          {t('rules.add')}
        </Button>
      </div>

      {!loading && rules.length > 0 && !hasMatchRule && (
        <div className="flex items-center gap-2 rounded-md border border-yellow-400 bg-yellow-50 px-4 py-3 text-sm text-yellow-800 dark:border-yellow-600 dark:bg-yellow-950 dark:text-yellow-200">
          <AlertTriangleIcon className="h-4 w-4 flex-shrink-0" />
          <span>{t('rules.noMatchRule')}</span>
        </div>
      )}

      <div className="flex items-center gap-2">
        <Input
          type="text"
          placeholder={t('rules.searchPlaceholder')}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="max-w-xs"
        />
        <Select
          value={typeFilter}
          onValueChange={(v) => setTypeFilter(!v || v === '__all__' ? '' : v)}
        >
          <SelectTrigger className="w-44">
            <SelectValue placeholder={t('common.allTypes')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">{t('common.allTypes')}</SelectItem>
            {RULE_TYPES.map((rt) => (
              <SelectItem key={rt} value={rt}>
                {rt}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {loading ? (
        <div className="text-center py-12 text-muted-foreground">{t('common.loading')}</div>
      ) : (
        <RuleList
          rules={rules}
          onEdit={handleEditClick}
          onDeleted={fetchRules}
          onReorder={handleReorder}
        />
      )}

      <RuleForm
        open={formOpen}
        onOpenChange={(o) => {
          setFormOpen(o);
          if (!o) setEditingRule(undefined);
        }}
        rule={editingRule}
        onSave={handleSave}
      />
    </div>
  );
}
