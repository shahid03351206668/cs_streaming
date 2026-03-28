"use client";

import { useEffect, useState } from "react";
import { Plus, Pencil, Trash2, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useToast } from "@/components/ui/use-toast";
import {
  getPromotions,
  createPromotion,
  updatePromotion,
  deletePromotion,
  CreateOfferParams,
} from "@/lib/api";
import { DataPagination } from "@/components/ui/data-pagination";

interface Promotion extends CreateOfferParams {
  id: string;
  created_at?: string;
}

const defaultForm: CreateOfferParams = {
  name: "",
  description: "",
  min_jobs_completed: 0,
  min_jobs_posted: 0,
  discount_percentage: 0,
  discount_amount: 0,
  max_discount_amount: 0,
  is_active: true,
  expires_at: "",
};

export default function PromotionsPage() {
  const [promotions, setPromotions] = useState<Promotion[]>([]);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [editingPromotion, setEditingPromotion] = useState<Promotion | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [formData, setFormData] = useState<CreateOfferParams>(defaultForm);
  const [submitting, setSubmitting] = useState(false);
  const { toast } = useToast();

  const fetchPromotions = async () => {
    try {
      const res = await getPromotions();
      const data = Array.isArray(res.data) ? res.data : (res.data?.data || []);
      setPromotions(data);
    } catch {
      toast({
        variant: "destructive",
        title: "Error",
        description: "Failed to load promotions.",
      });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchPromotions();
  }, []);

  const openCreateDialog = () => {
    setEditingPromotion(null);
    setFormData(defaultForm);
    setDialogOpen(true);
  };

  const openEditDialog = (promotion: Promotion) => {
    setEditingPromotion(promotion);
    setFormData({
      name: promotion.name,
      description: promotion.description,
      min_jobs_completed: promotion.min_jobs_completed,
      min_jobs_posted: promotion.min_jobs_posted,
      discount_percentage: promotion.discount_percentage,
      discount_amount: promotion.discount_amount,
      max_discount_amount: promotion.max_discount_amount,
      is_active: promotion.is_active,
      expires_at: promotion.expires_at || "",
    });
    setDialogOpen(true);
  };

  const closeDialog = () => {
    setDialogOpen(false);
    setEditingPromotion(null);
    setFormData(defaultForm);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);

    const payload: CreateOfferParams = {
      ...formData,
      expires_at: formData.expires_at || undefined,
    };

    try {
      if (editingPromotion) {
        await updatePromotion(editingPromotion.id, payload);
        toast({ title: "Success", description: "Promotion updated successfully." });
      } else {
        await createPromotion(payload);
        toast({ title: "Success", description: "Promotion created successfully." });
      }
      closeDialog();
      await fetchPromotions();
    } catch {
      toast({
        variant: "destructive",
        title: "Error",
        description: editingPromotion
          ? "Failed to update promotion."
          : "Failed to create promotion.",
      });
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (!deletingId) return;
    try {
      await deletePromotion(deletingId);
      toast({ title: "Success", description: "Promotion deleted successfully." });
      await fetchPromotions();
    } catch {
      toast({
        variant: "destructive",
        title: "Error",
        description: "Failed to delete promotion.",
      });
    } finally {
      setDeleteDialogOpen(false);
      setDeletingId(null);
    }
  };

  const handleToggleActive = async (promotion: Promotion) => {
    try {
      await updatePromotion(promotion.id, {
        ...promotion,
        is_active: !promotion.is_active,
      });
      toast({
        title: "Updated",
        description: `Promotion ${!promotion.is_active ? "activated" : "deactivated"}.`,
      });
      await fetchPromotions();
    } catch {
      toast({
        variant: "destructive",
        title: "Error",
        description: "Failed to update promotion status.",
      });
    }
  };

  const formatCurrency = (pence: number) => `£${(pence / 100).toFixed(2)}`;
  const paginated = promotions.slice((page - 1) * pageSize, page * pageSize);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">Promotions</h2>
          <p className="text-muted-foreground">
            Manage promotional offers and discounts.
          </p>
        </div>
        <Button onClick={openCreateDialog} className="gap-2">
          <Plus className="h-4 w-4" />
          New Promotion
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">All Promotions</CardTitle>
          <CardDescription>
            {loading ? "Loading..." : `${promotions.length} promotion${promotions.length !== 1 ? "s" : ""} found`}
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {loading ? (
            <div className="p-6 space-y-3">
              {Array.from({ length: 4 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : promotions.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-center">
              <p className="text-muted-foreground">No promotions found.</p>
              <Button
                variant="outline"
                onClick={openCreateDialog}
                className="mt-4 gap-2"
              >
                <Plus className="h-4 w-4" />
                Create your first promotion
              </Button>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Description</TableHead>
                  <TableHead className="text-right">Min Jobs</TableHead>
                  <TableHead className="text-right">Discount %</TableHead>
                  <TableHead className="text-right">Discount Amt</TableHead>
                  <TableHead className="text-right">Max Discount</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Expires</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {paginated.map((promo) => (
                  <TableRow key={promo.id}>
                    <TableCell className="font-medium">{promo.name}</TableCell>
                    <TableCell className="max-w-[200px] truncate text-muted-foreground">
                      {promo.description || "—"}
                    </TableCell>
                    <TableCell className="text-right">
                      <span className="text-xs">
                        C:{promo.min_jobs_completed} / P:{promo.min_jobs_posted}
                      </span>
                    </TableCell>
                    <TableCell className="text-right">
                      {promo.discount_percentage > 0
                        ? `${promo.discount_percentage}%`
                        : "—"}
                    </TableCell>
                    <TableCell className="text-right">
                      {promo.discount_amount > 0
                        ? formatCurrency(promo.discount_amount)
                        : "—"}
                    </TableCell>
                    <TableCell className="text-right">
                      {promo.max_discount_amount > 0
                        ? formatCurrency(promo.max_discount_amount)
                        : "—"}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Switch
                          checked={promo.is_active}
                          onCheckedChange={() => handleToggleActive(promo)}
                        />
                        <Badge variant={promo.is_active ? "success" : "secondary"}>
                          {promo.is_active ? "Active" : "Inactive"}
                        </Badge>
                      </div>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {promo.expires_at
                        ? new Date(promo.expires_at).toLocaleDateString()
                        : "Never"}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => openEditDialog(promo)}
                        >
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="text-red-500 hover:text-red-700 hover:bg-red-50"
                          onClick={() => {
                            setDeletingId(promo.id);
                            setDeleteDialogOpen(true);
                          }}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
          {!loading && promotions.length > 0 && (
            <DataPagination
              total={promotions.length}
              page={page}
              pageSize={pageSize}
              onPageChange={setPage}
              onPageSizeChange={setPageSize}
            />
          )}
        </CardContent>
      </Card>

      {/* Create / Edit Dialog */}
      <Dialog open={dialogOpen} onOpenChange={(open) => !open && closeDialog()}>
        <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              {editingPromotion ? "Edit Promotion" : "New Promotion"}
            </DialogTitle>
            <DialogDescription>
              {editingPromotion
                ? "Update the promotional offer details."
                : "Create a new promotional offer for users."}
            </DialogDescription>
          </DialogHeader>

          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="sm:col-span-2 space-y-2">
                <Label htmlFor="name">Name *</Label>
                <Input
                  id="name"
                  value={formData.name}
                  onChange={(e) =>
                    setFormData({ ...formData, name: e.target.value })
                  }
                  placeholder="Summer Sale"
                  required
                />
              </div>

              <div className="sm:col-span-2 space-y-2">
                <Label htmlFor="description">Description</Label>
                <Input
                  id="description"
                  value={formData.description}
                  onChange={(e) =>
                    setFormData({ ...formData, description: e.target.value })
                  }
                  placeholder="Get 20% off on your next job"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="min_jobs_completed">Min Jobs Completed</Label>
                <Input
                  id="min_jobs_completed"
                  type="number"
                  min={0}
                  value={formData.min_jobs_completed}
                  onChange={(e) =>
                    setFormData({
                      ...formData,
                      min_jobs_completed: Number(e.target.value),
                    })
                  }
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="min_jobs_posted">Min Jobs Posted</Label>
                <Input
                  id="min_jobs_posted"
                  type="number"
                  min={0}
                  value={formData.min_jobs_posted}
                  onChange={(e) =>
                    setFormData({
                      ...formData,
                      min_jobs_posted: Number(e.target.value),
                    })
                  }
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="discount_percentage">Discount Percentage (0-100)</Label>
                <Input
                  id="discount_percentage"
                  type="number"
                  min={0}
                  max={100}
                  value={formData.discount_percentage}
                  onChange={(e) =>
                    setFormData({
                      ...formData,
                      discount_percentage: Number(e.target.value),
                    })
                  }
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="discount_amount">Discount Amount (pence)</Label>
                <Input
                  id="discount_amount"
                  type="number"
                  min={0}
                  value={formData.discount_amount}
                  onChange={(e) =>
                    setFormData({
                      ...formData,
                      discount_amount: Number(e.target.value),
                    })
                  }
                  placeholder="500 = £5.00"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="max_discount_amount">Max Discount Amount (pence)</Label>
                <Input
                  id="max_discount_amount"
                  type="number"
                  min={0}
                  value={formData.max_discount_amount}
                  onChange={(e) =>
                    setFormData({
                      ...formData,
                      max_discount_amount: Number(e.target.value),
                    })
                  }
                  placeholder="1000 = £10.00"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="expires_at">Expires At</Label>
                <Input
                  id="expires_at"
                  type="datetime-local"
                  value={formData.expires_at || ""}
                  onChange={(e) =>
                    setFormData({ ...formData, expires_at: e.target.value })
                  }
                />
              </div>

              <div className="flex items-center gap-3 pt-6">
                <Switch
                  id="is_active"
                  checked={formData.is_active}
                  onCheckedChange={(checked) =>
                    setFormData({ ...formData, is_active: checked })
                  }
                />
                <Label htmlFor="is_active">Active</Label>
              </div>
            </div>

            <DialogFooter className="gap-2">
              <Button type="button" variant="outline" onClick={closeDialog}>
                Cancel
              </Button>
              <Button type="submit" disabled={submitting}>
                {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {editingPromotion ? "Update Promotion" : "Create Promotion"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Delete Promotion</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete this promotion? This action cannot
              be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2">
            <Button
              variant="outline"
              onClick={() => {
                setDeleteDialogOpen(false);
                setDeletingId(null);
              }}
            >
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDelete}>
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
