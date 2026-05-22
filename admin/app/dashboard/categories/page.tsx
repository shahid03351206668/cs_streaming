"use client";

import { useEffect, useRef, useState } from "react";
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
  FolderOpen,
  GripVertical,
  Loader2,
  Pencil,
  Plus,
  Save,
  Trash2,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useToast } from "@/components/ui/use-toast";
import {
  createCategory,
  deleteCategory,
  getCategories,
  reorderCategories,
  updateCategory,
} from "@/lib/api";

// ─── Types ────────────────────────────────────────────────────────────────────

interface Category {
  id: string;
  name: string;
  sort_order: number;
  created_at?: string;
}

// ─── Drag-overlay ghost card ──────────────────────────────────────────────────

function DragGhost({ name, order }: { name: string; order: number }) {
  return (
    <div className="flex items-center gap-3 rounded-md border bg-white px-4 py-3 shadow-xl ring-2 ring-primary/20">
      <GripVertical className="h-4 w-4 shrink-0 text-muted-foreground" />
      <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">
        {order}
      </span>
      <span className="flex-1 font-medium">{name}</span>
    </div>
  );
}

// ─── Single sortable row ──────────────────────────────────────────────────────

interface RowProps {
  category: Category;
  index: number;
  onEdit: (cat: Category) => void;
  onDelete: (cat: Category) => void;
}

function SortableRow({ category, index, onEdit, onDelete }: RowProps) {
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
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`flex items-center gap-3 rounded-md border bg-card px-4 py-3 transition-shadow ${
        isDragging ? "opacity-40 shadow-none" : "shadow-sm hover:shadow-md"
      }`}
    >
      {/* Drag handle */}
      <button
        {...listeners}
        {...attributes}
        className="cursor-grab touch-none text-muted-foreground/40 hover:text-muted-foreground active:cursor-grabbing focus:outline-none"
        tabIndex={-1}
        aria-label="Drag to reorder"
      >
        <GripVertical className="h-4 w-4" />
      </button>

      {/* Order badge */}
      <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">
        {index + 1}
      </span>

      {/* Name */}
      <span className="flex-1 font-medium">{category.name}</span>

      {/* Created date */}
      <span className="hidden text-xs text-muted-foreground sm:block">
        {category.created_at
          ? new Date(category.created_at).toLocaleDateString()
          : "—"}
      </span>

      {/* Actions */}
      <div className="flex items-center gap-1">
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8"
          onClick={() => onEdit(category)}
          aria-label="Edit"
        >
          <Pencil className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8 text-red-500 hover:bg-red-50 hover:text-red-600"
          onClick={() => onDelete(category)}
          aria-label="Delete"
        >
          <Trash2 className="h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  );
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function CategoriesPage() {
  const [categories, setCategories] = useState<Category[]>([]);
  const [loading, setLoading] = useState(true);

  // Drag state
  const [activeId, setActiveId] = useState<string | null>(null);
  const activeCat = categories.find((c) => c.id === activeId) ?? null;
  const activeIndex = categories.findIndex((c) => c.id === activeId);

  // Save-order state
  const [orderDirty, setOrderDirty] = useState(false);
  const [savingOrder, setSavingOrder] = useState(false);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Create / edit dialog
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingCat, setEditingCat] = useState<Category | null>(null);
  const [name, setName] = useState("");
  const [submitting, setSubmitting] = useState(false);

  // Delete dialog
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deletingCat, setDeletingCat] = useState<Category | null>(null);
  const [deleting, setDeleting] = useState(false);

  const { toast } = useToast();

  // ── Load ────────────────────────────────────────────────────────────────────
  const load = async () => {
    try {
      const res = await getCategories();
      const raw: Category[] = Array.isArray(res.data)
        ? res.data
        : Array.isArray(res.data?.data)
        ? res.data.data
        : [];
      setCategories([...raw].sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0)));
    } catch {
      toast({ variant: "destructive", title: "Error", description: "Failed to load categories." });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  // ── dnd-kit sensors ──────────────────────────────────────────────────────────
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  );

  // ── Drag handlers ────────────────────────────────────────────────────────────
  const handleDragStart = ({ active }: DragStartEvent) => {
    setActiveId(active.id as string);
  };

  const handleDragEnd = ({ active, over }: DragEndEvent) => {
    setActiveId(null);
    if (!over || active.id === over.id) return;

    setCategories((prev) => {
      const from = prev.findIndex((c) => c.id === active.id);
      const to   = prev.findIndex((c) => c.id === over.id);
      const next = arrayMove(prev, from, to).map((c, i) => ({ ...c, sort_order: i }));
      scheduleSave(next);
      return next;
    });
    setOrderDirty(true);
  };

  const scheduleSave = (ordered: Category[]) => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => persistOrder(ordered), 700);
  };

  const persistOrder = async (ordered: Category[]) => {
    setSavingOrder(true);
    try {
      await reorderCategories(ordered.map((c) => c.id));
      setOrderDirty(false);
    } catch {
      toast({ variant: "destructive", title: "Error", description: "Failed to save order." });
    } finally {
      setSavingOrder(false);
    }
  };

  const saveOrderNow = () => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    persistOrder(categories);
  };

  // ── Create / edit ────────────────────────────────────────────────────────────
  const openCreate = () => { setEditingCat(null); setName(""); setDialogOpen(true); };
  const openEdit   = (cat: Category) => { setEditingCat(cat); setName(cat.name); setDialogOpen(true); };
  const closeDialog = () => { setDialogOpen(false); setEditingCat(null); setName(""); };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    setSubmitting(true);
    try {
      if (editingCat) {
        await updateCategory(editingCat.id, { name: name.trim() });
        toast({ title: "Updated", description: "Category name saved." });
      } else {
        await createCategory({ name: name.trim() });
        toast({ title: "Created", description: "Category added." });
      }
      closeDialog();
      await load();
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error;
      toast({ variant: "destructive", title: "Error", description: msg ?? "Something went wrong." });
    } finally {
      setSubmitting(false);
    }
  };

  // ── Delete ───────────────────────────────────────────────────────────────────
  const openDelete = (cat: Category) => { setDeletingCat(cat); setDeleteDialogOpen(true); };

  const handleDelete = async () => {
    if (!deletingCat) return;
    setDeleting(true);
    try {
      await deleteCategory(deletingCat.id);
      toast({ title: "Deleted", description: "Category removed." });
      await load();
    } catch {
      toast({ variant: "destructive", title: "Error", description: "Failed to delete category." });
    } finally {
      setDeleting(false);
      setDeleteDialogOpen(false);
      setDeletingCat(null);
    }
  };

  // ── Render ───────────────────────────────────────────────────────────────────
  return (
    <div className="space-y-6">
      {/* Page header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">Categories</h2>
          <p className="text-muted-foreground">
            Manage job categories. Drag rows to set the display order.
          </p>
        </div>
        <Button onClick={openCreate} className="gap-2">
          <Plus className="h-4 w-4" />
          New Category
        </Button>
      </div>

      <Card>
        <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-3">
          <div>
            <CardTitle className="text-base">All Categories</CardTitle>
            <CardDescription>
              {loading
                ? "Loading…"
                : `${categories.length} categor${categories.length !== 1 ? "ies" : "y"}`}
            </CardDescription>
          </div>

          {/* Save-order button — appears when a drag has happened */}
          {(orderDirty || savingOrder) && (
            <Button
              size="sm"
              variant="outline"
              onClick={saveOrderNow}
              disabled={savingOrder}
              className="gap-1.5"
            >
              {savingOrder ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Save className="h-3.5 w-3.5" />
              )}
              {savingOrder ? "Saving…" : "Save order"}
            </Button>
          )}
        </CardHeader>

        <CardContent className="pt-0">
          {loading ? (
            <div className="space-y-2">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-14 w-full rounded-md" />
              ))}
            </div>
          ) : categories.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-center">
              <FolderOpen className="mb-3 h-10 w-10 text-muted-foreground" />
              <p className="text-muted-foreground">No categories yet.</p>
              <Button variant="outline" onClick={openCreate} className="mt-4 gap-2">
                <Plus className="h-4 w-4" />
                Create first category
              </Button>
            </div>
          ) : (
            <>
              {/* Column labels */}
              <div className="mb-1.5 flex items-center gap-3 px-4 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                <span className="w-4" />
                <span className="w-6" />
                <span className="flex-1">Name</span>
                <span className="hidden sm:block">Created</span>
                <span className="w-20 text-right">Actions</span>
              </div>

              {/* Drag context */}
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
                    {categories.map((cat, idx) => (
                      <SortableRow
                        key={cat.id}
                        category={cat}
                        index={idx}
                        onEdit={openEdit}
                        onDelete={openDelete}
                      />
                    ))}
                  </div>
                </SortableContext>

                {/* Ghost card under the pointer while dragging */}
                <DragOverlay dropAnimation={{ duration: 150, easing: "ease" }}>
                  {activeCat && (
                    <DragGhost name={activeCat.name} order={activeIndex + 1} />
                  )}
                </DragOverlay>
              </DndContext>

              <p className="mt-3 text-center text-xs text-muted-foreground">
                Drag <GripVertical className="inline h-3 w-3" /> to reorder · order is saved automatically
              </p>
            </>
          )}
        </CardContent>
      </Card>

      {/* ── Create / Edit dialog ─────────────────────────────────────────────── */}
      <Dialog open={dialogOpen} onOpenChange={(open) => !open && closeDialog()}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{editingCat ? "Edit Category" : "New Category"}</DialogTitle>
            <DialogDescription>
              {editingCat ? "Update the category name." : "Add a new job category."}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="cat-name">Name *</Label>
              <Input
                id="cat-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. Web Development"
                required
                autoFocus
              />
            </div>
            <DialogFooter className="gap-2">
              <Button type="button" variant="outline" onClick={closeDialog}>
                Cancel
              </Button>
              <Button type="submit" disabled={submitting || !name.trim()}>
                {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {editingCat ? "Update" : "Create"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* ── Delete confirm dialog ────────────────────────────────────────────── */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Delete Category</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete{" "}
              <span className="font-semibold">&ldquo;{deletingCat?.name}&rdquo;</span>?
              This cannot be undone and may affect existing jobs.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2">
            <Button
              variant="outline"
              onClick={() => { setDeleteDialogOpen(false); setDeletingCat(null); }}
            >
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDelete} disabled={deleting}>
              {deleting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
