"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  DndContext,
  DragEndEvent,
  DragOverlay,
  DragStartEvent,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import {
  CheckIcon,
  GripVerticalIcon,
  Loader2Icon,
  PencilIcon,
  PlusIcon,
  SaveIcon,
  Trash2Icon,
  XIcon,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Skeleton } from "@/components/ui/skeleton";

import {
  Category,
  createCategory,
  deleteCategory,
  fetchCategories,
  reorderCategories,
  updateCategory,
} from "@/lib/categories-api";

// ─── Single draggable row ─────────────────────────────────────────────────────

interface SortableRowProps {
  category: Category;
  isEditing: boolean;
  editName: string;
  isSaving: boolean;
  onEditStart: (cat: Category) => void;
  onEditChange: (val: string) => void;
  onEditSave: () => void;
  onEditCancel: () => void;
  onDelete: (id: string) => void;
}

function SortableRow({
  category,
  isEditing,
  editName,
  isSaving,
  onEditStart,
  onEditChange,
  onEditSave,
  onEditCancel,
  onDelete,
}: SortableRowProps) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: category.id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.4 : 1,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className="flex items-center gap-3 rounded-md border bg-card px-3 py-2.5 shadow-sm"
    >
      {/* Drag handle */}
      <button
        {...listeners}
        {...attributes}
        className="cursor-grab touch-none text-muted-foreground/50 hover:text-muted-foreground active:cursor-grabbing"
        aria-label="Drag to reorder"
        tabIndex={-1}
      >
        <GripVerticalIcon className="h-4 w-4" />
      </button>

      {/* Sort order badge */}
      <Badge variant="secondary" className="w-8 justify-center font-mono text-xs shrink-0">
        {category.sort_order + 1}
      </Badge>

      {/* Name / inline edit */}
      {isEditing ? (
        <div className="flex flex-1 items-center gap-2">
          <Input
            value={editName}
            onChange={(e) => onEditChange(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") onEditSave();
              if (e.key === "Escape") onEditCancel();
            }}
            className="h-7 flex-1 text-sm"
            autoFocus
          />
          <Button
            size="icon"
            variant="ghost"
            className="h-7 w-7 text-green-600 hover:text-green-700"
            onClick={onEditSave}
            disabled={isSaving || !editName.trim()}
            aria-label="Save"
          >
            {isSaving ? (
              <Loader2Icon className="h-4 w-4 animate-spin" />
            ) : (
              <CheckIcon className="h-4 w-4" />
            )}
          </Button>
          <Button
            size="icon"
            variant="ghost"
            className="h-7 w-7"
            onClick={onEditCancel}
            aria-label="Cancel"
          >
            <XIcon className="h-4 w-4" />
          </Button>
        </div>
      ) : (
        <>
          <span className="flex-1 text-sm font-medium">{category.name}</span>

          <Button
            size="icon"
            variant="ghost"
            className="h-7 w-7"
            onClick={() => onEditStart(category)}
            aria-label="Edit"
          >
            <PencilIcon className="h-3.5 w-3.5" />
          </Button>

          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button
                size="icon"
                variant="ghost"
                className="h-7 w-7 text-destructive hover:text-destructive"
                aria-label="Delete"
              >
                <Trash2Icon className="h-3.5 w-3.5" />
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Delete category?</AlertDialogTitle>
                <AlertDialogDescription>
                  &ldquo;{category.name}&rdquo; will be permanently removed. Jobs
                  using this category will lose their category reference.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  onClick={() => onDelete(category.id)}
                  className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                >
                  Delete
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </>
      )}
    </div>
  );
}

// ─── Drag overlay clone (shown while dragging) ────────────────────────────────

function DragOverlayRow({ category }: { category: Category }) {
  return (
    <div className="flex items-center gap-3 rounded-md border bg-card px-3 py-2.5 shadow-lg ring-2 ring-primary/30">
      <GripVerticalIcon className="h-4 w-4 text-muted-foreground" />
      <Badge variant="secondary" className="w-8 justify-center font-mono text-xs">
        {category.sort_order + 1}
      </Badge>
      <span className="flex-1 text-sm font-medium">{category.name}</span>
    </div>
  );
}

// ─── Main component ───────────────────────────────────────────────────────────

export function CategoryList() {
  const [categories, setCategories] = useState<Category[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Drag state
  const [activeId, setActiveId] = useState<string | null>(null);
  const activeCategory = categories.find((c) => c.id === activeId) ?? null;

  // Inline edit state
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editName, setEditName] = useState("");
  const [editSaving, setEditSaving] = useState(false);

  // New category form
  const [newName, setNewName] = useState("");
  const [creating, setCreating] = useState(false);

  // Pending reorder — debounced before sending to API
  const [reorderDirty, setReorderDirty] = useState(false);
  const [reorderSaving, setReorderSaving] = useState(false);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // ── Load ────────────────────────────────────────────────────────────────────
  const load = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await fetchCategories();
      setCategories(data.sort((a, b) => a.sort_order - b.sort_order));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  // ── Sensors (pointer + keyboard) ────────────────────────────────────────────
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    })
  );

  // ── Drag handlers ────────────────────────────────────────────────────────────
  const handleDragStart = ({ active }: DragStartEvent) => {
    setActiveId(active.id as string);
    // Cancel any in-progress inline edit when the user starts dragging
    setEditingId(null);
  };

  const handleDragEnd = ({ active, over }: DragEndEvent) => {
    setActiveId(null);
    if (!over || active.id === over.id) return;

    setCategories((prev) => {
      const oldIdx = prev.findIndex((c) => c.id === active.id);
      const newIdx = prev.findIndex((c) => c.id === over.id);
      const reordered = arrayMove(prev, oldIdx, newIdx).map((c, i) => ({
        ...c,
        sort_order: i,
      }));
      scheduleReorder(reordered);
      return reordered;
    });
    setReorderDirty(true);
  };

  // Debounce the API call so rapid drags don't flood the server
  const scheduleReorder = (ordered: Category[]) => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      setReorderSaving(true);
      try {
        await reorderCategories(ordered.map((c) => c.id));
        setReorderDirty(false);
      } catch {
        // Silent — the optimistic update already reflects the correct order
      } finally {
        setReorderSaving(false);
      }
    }, 600);
  };

  // ── Save order manually ──────────────────────────────────────────────────────
  const saveOrder = async () => {
    setReorderSaving(true);
    try {
      await reorderCategories(categories.map((c) => c.id));
      setReorderDirty(false);
    } finally {
      setReorderSaving(false);
    }
  };

  // ── Inline edit ──────────────────────────────────────────────────────────────
  const startEdit = (cat: Category) => {
    setEditingId(cat.id);
    setEditName(cat.name);
  };

  const saveEdit = async () => {
    if (!editingId || !editName.trim()) return;
    setEditSaving(true);
    try {
      const updated = await updateCategory(editingId, { name: editName.trim() });
      setCategories((prev) =>
        prev.map((c) => (c.id === editingId ? { ...c, name: updated.name } : c))
      );
      setEditingId(null);
    } catch (e) {
      alert((e as Error).message);
    } finally {
      setEditSaving(false);
    }
  };

  // ── Create ───────────────────────────────────────────────────────────────────
  const handleCreate = async () => {
    if (!newName.trim()) return;
    setCreating(true);
    try {
      const cat = await createCategory(newName.trim());
      setCategories((prev) => [...prev, cat]);
      setNewName("");
    } catch (e) {
      alert((e as Error).message);
    } finally {
      setCreating(false);
    }
  };

  // ── Delete ───────────────────────────────────────────────────────────────────
  const handleDelete = async (id: string) => {
    try {
      await deleteCategory(id);
      setCategories((prev) => prev.filter((c) => c.id !== id));
    } catch (e) {
      alert((e as Error).message);
    }
  };

  // ── Render ───────────────────────────────────────────────────────────────────
  if (loading) {
    return (
      <div className="space-y-2">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="h-12 w-full rounded-md" />
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-md border border-destructive/40 bg-destructive/10 p-4 text-sm text-destructive">
        {error}{" "}
        <button onClick={load} className="underline">
          Retry
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Header row */}
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          Drag rows to reorder. Changes are saved automatically.
        </p>
        {reorderDirty && (
          <Button
            size="sm"
            variant="outline"
            onClick={saveOrder}
            disabled={reorderSaving}
            className="gap-1.5"
          >
            {reorderSaving ? (
              <Loader2Icon className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <SaveIcon className="h-3.5 w-3.5" />
            )}
            Save order
          </Button>
        )}
        {reorderSaving && !reorderDirty && (
          <span className="flex items-center gap-1 text-xs text-muted-foreground">
            <Loader2Icon className="h-3 w-3 animate-spin" />
            Saving…
          </span>
        )}
      </div>

      {/* Sortable list */}
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragStart={handleDragStart}
        onDragEnd={handleDragEnd}
      >
        <SortableContext
          items={categories.map((c) => c.id)}
          strategy={verticalListSortingStrategy}
        >
          <div className="space-y-1.5">
            {categories.map((cat) => (
              <SortableRow
                key={cat.id}
                category={cat}
                isEditing={editingId === cat.id}
                editName={editName}
                isSaving={editSaving}
                onEditStart={startEdit}
                onEditChange={setEditName}
                onEditSave={saveEdit}
                onEditCancel={() => setEditingId(null)}
                onDelete={handleDelete}
              />
            ))}
          </div>
        </SortableContext>

        {/* Ghost shown under the pointer while dragging */}
        <DragOverlay dropAnimation={{ duration: 150, easing: "ease" }}>
          {activeCategory && <DragOverlayRow category={activeCategory} />}
        </DragOverlay>
      </DndContext>

      {categories.length === 0 && (
        <p className="py-8 text-center text-sm text-muted-foreground">
          No categories yet. Add one below.
        </p>
      )}

      {/* Add new category */}
      <div className="flex gap-2 pt-2 border-t">
        <Input
          placeholder="New category name…"
          value={newName}
          onChange={(e) => setNewName(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && handleCreate()}
          className="flex-1"
        />
        <Button
          onClick={handleCreate}
          disabled={creating || !newName.trim()}
          className="gap-1.5"
        >
          {creating ? (
            <Loader2Icon className="h-4 w-4 animate-spin" />
          ) : (
            <PlusIcon className="h-4 w-4" />
          )}
          Add
        </Button>
      </div>
    </div>
  );
}
