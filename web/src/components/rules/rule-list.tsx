import { useState } from 'react';
import { toast } from 'sonner';
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { GripVerticalIcon, PencilIcon, Trash2Icon } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { ConfirmDialog } from '@/components/shared/confirm-dialog';
import type { Rule } from '@/api/rules';
import { rulesApi } from '@/api/rules';
import { useT } from '@/i18n';

interface SortableRuleRowProps {
  rule: Rule;
  index: number;
  onEdit: (rule: Rule) => void;
  onDeleteClick: (rule: Rule) => void;
}

function SortableRuleRow({ rule, index, onEdit, onDeleteClick }: SortableRuleRowProps) {
  const t = useT();
  const isMatch = rule.type === 'MATCH';
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: rule.id,
    disabled: isMatch,
  });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };

  const noResolve = rule.params?.['no-resolve'];

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`flex items-center gap-3 rounded-md border px-3 py-2 ${
        isMatch ? 'bg-muted/50 border-dashed' : 'bg-background'
      }`}
    >
      {isMatch ? (
        <span className="w-5 flex-shrink-0" />
      ) : (
        <button
          type="button"
          className="cursor-grab text-muted-foreground hover:text-foreground touch-none flex-shrink-0"
          {...attributes}
          {...listeners}
        >
          <GripVerticalIcon className="h-4 w-4" />
        </button>
      )}

      <span className="w-8 text-xs text-muted-foreground text-right flex-shrink-0">{index + 1}</span>

      <Badge variant={isMatch ? 'secondary' : 'default'} className="flex-shrink-0 font-mono text-xs">
        {rule.type}
      </Badge>

      <span className="flex-1 text-sm font-mono truncate">
        {rule.payload || <span className="text-muted-foreground italic">—</span>}
      </span>

      <span className="text-sm text-muted-foreground flex-shrink-0">{rule.target}</span>

      {noResolve && (
        <Badge variant="outline" className="text-xs flex-shrink-0">
          no-resolve
        </Badge>
      )}

      <div className="flex items-center gap-1 flex-shrink-0">
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => onEdit(rule)}
          title={t('common.edit')}
        >
          <PencilIcon className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => onDeleteClick(rule)}
          title={t('common.delete')}
        >
          <Trash2Icon className="h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  );
}

interface RuleListProps {
  rules: Rule[];
  onEdit: (rule: Rule) => void;
  onDeleted: () => void;
  onReorder: (rules: Rule[]) => void;
}

export function RuleList({ rules, onEdit, onDeleted, onReorder }: RuleListProps) {
  const t = useT();
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);

  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  );

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;

    // Don't allow dragging over MATCH rule position
    const activeRule = rules.find((r) => r.id === String(active.id));
    const overRule = rules.find((r) => r.id === String(over.id));
    if (!activeRule || !overRule) return;
    if (overRule.type === 'MATCH') return;

    const oldIndex = rules.indexOf(activeRule);
    const newIndex = rules.indexOf(overRule);
    const newRules = arrayMove(rules, oldIndex, newIndex);
    onReorder(newRules);
  };

  const handleDeleteClick = (rule: Rule) => {
    setDeletingId(rule.id);
    setConfirmOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!deletingId) return;
    try {
      await rulesApi.delete(deletingId);
      toast.success(t('rules.deleted'));
      onDeleted();
    } catch {
      toast.error(t('rules.deleteFailed'));
    } finally {
      setDeletingId(null);
    }
  };

  if (rules.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        {t('rules.noRules')}
      </div>
    );
  }

  const sortableIds = rules.filter((r) => r.type !== 'MATCH').map((r) => r.id);

  return (
    <>
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
        <SortableContext items={sortableIds} strategy={verticalListSortingStrategy}>
          <div className="space-y-1">
            {rules.map((rule, index) => (
              <SortableRuleRow
                key={rule.id}
                rule={rule}
                index={index}
                onEdit={onEdit}
                onDeleteClick={handleDeleteClick}
              />
            ))}
          </div>
        </SortableContext>
      </DndContext>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('rules.deleteTitle')}
        description={t('rules.deleteConfirm')}
        onConfirm={handleDeleteConfirm}
        variant="destructive"
      />
    </>
  );
}
