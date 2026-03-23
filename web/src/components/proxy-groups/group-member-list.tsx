import { useState } from 'react';
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
import { GripVerticalIcon, XIcon, PlusIcon } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

interface SortableMemberProps {
  id: string;
  onRemove: (name: string) => void;
}

function SortableMember({ id, onRemove }: SortableMemberProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
  });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className="flex items-center gap-2 rounded-md border bg-background px-3 py-2"
    >
      <button
        type="button"
        className="cursor-grab text-muted-foreground hover:text-foreground touch-none"
        {...attributes}
        {...listeners}
      >
        <GripVerticalIcon className="h-4 w-4" />
      </button>
      <span className="flex-1 text-sm font-medium">{id}</span>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        onClick={() => onRemove(id)}
        title="Remove"
      >
        <XIcon className="h-4 w-4" />
      </Button>
    </div>
  );
}

interface GroupMemberListProps {
  members: string[];
  onMembersChange: (members: string[]) => void;
  availableItems: string[];
}

export function GroupMemberList({ members, onMembersChange, availableItems }: GroupMemberListProps) {
  const [addValue, setAddValue] = useState('');

  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  );

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (over && active.id !== over.id) {
      const oldIndex = members.indexOf(String(active.id));
      const newIndex = members.indexOf(String(over.id));
      onMembersChange(arrayMove(members, oldIndex, newIndex));
    }
  };

  const handleAdd = (value: string | null) => {
    if (value && !members.includes(value)) {
      onMembersChange([...members, value]);
    }
    setAddValue('');
  };

  const handleRemove = (name: string) => {
    onMembersChange(members.filter((m) => m !== name));
  };

  const unselectedItems = availableItems.filter((item) => !members.includes(item));

  return (
    <div className="space-y-2">
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
        <SortableContext items={members} strategy={verticalListSortingStrategy}>
          <div className="space-y-1 min-h-[2rem]">
            {members.length === 0 && (
              <div className="text-sm text-muted-foreground py-2 text-center border rounded-md">
                No members added yet
              </div>
            )}
            {members.map((member) => (
              <SortableMember key={member} id={member} onRemove={handleRemove} />
            ))}
          </div>
        </SortableContext>
      </DndContext>

      {unselectedItems.length > 0 && (
        <div className="flex items-center gap-2">
          <Select value={addValue} onValueChange={handleAdd}>
            <SelectTrigger className="flex-1">
              <SelectValue placeholder="Add a member..." />
            </SelectTrigger>
            <SelectContent>
              {unselectedItems.map((item) => (
                <SelectItem key={item} value={item}>
                  {item}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            type="button"
            variant="outline"
            size="icon-sm"
            onClick={() => addValue && handleAdd(addValue)}
            title="Add member"
          >
            <PlusIcon className="h-4 w-4" />
          </Button>
        </div>
      )}
    </div>
  );
}
