"use client";

import { useEffect, useState } from "react";
import { Plus, Pencil, Trash2, Loader2, Building2 } from "lucide-react";
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
  Bank,
  BankParams,
  adminListBanks,
  adminCreateBank,
  adminUpdateBank,
  adminDeleteBank,
} from "@/lib/api";
import { DataPagination } from "@/components/ui/data-pagination";

const defaultForm: BankParams = {
  name: "",
  sort_code: "",
  logo_url: "",
  is_active: true,
};

export default function BanksPage() {
  const [banks, setBanks] = useState<Bank[]>([]);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [editingBank, setEditingBank] = useState<Bank | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [formData, setFormData] = useState<BankParams>(defaultForm);
  const [submitting, setSubmitting] = useState(false);
  const { toast } = useToast();

  const fetchBanks = async () => {
    try {
      const res = await adminListBanks();
      const data = Array.isArray(res.data) ? res.data : (res.data?.data ?? []);
      setBanks(data);
    } catch {
      toast({ variant: "destructive", title: "Error", description: "Failed to load banks." });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchBanks();
  }, []);

  const openCreateDialog = () => {
    setEditingBank(null);
    setFormData(defaultForm);
    setDialogOpen(true);
  };

  const openEditDialog = (bank: Bank) => {
    setEditingBank(bank);
    setFormData({
      name: bank.name,
      sort_code: bank.sort_code,
      logo_url: bank.logo_url,
      is_active: bank.is_active,
    });
    setDialogOpen(true);
  };

  const closeDialog = () => {
    setDialogOpen(false);
    setEditingBank(null);
    setFormData(defaultForm);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    try {
      if (editingBank) {
        await adminUpdateBank(editingBank.id, formData);
        toast({ title: "Updated", description: "Bank updated successfully." });
      } else {
        await adminCreateBank(formData);
        toast({ title: "Created", description: "Bank added successfully." });
      }
      closeDialog();
      await fetchBanks();
    } catch {
      toast({
        variant: "destructive",
        title: "Error",
        description: editingBank ? "Failed to update bank." : "Failed to create bank.",
      });
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (!deletingId) return;
    try {
      await adminDeleteBank(deletingId);
      toast({ title: "Deleted", description: "Bank removed successfully." });
      await fetchBanks();
    } catch {
      toast({ variant: "destructive", title: "Error", description: "Failed to delete bank." });
    } finally {
      setDeleteDialogOpen(false);
      setDeletingId(null);
    }
  };

  const handleToggleActive = async (bank: Bank) => {
    try {
      await adminUpdateBank(bank.id, { is_active: !bank.is_active });
      toast({ title: "Updated", description: `Bank ${!bank.is_active ? "activated" : "deactivated"}.` });
      await fetchBanks();
    } catch {
      toast({ variant: "destructive", title: "Error", description: "Failed to update bank status." });
    }
  };

  const paginated = banks.slice((page - 1) * pageSize, page * pageSize);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">Banks</h2>
          <p className="text-muted-foreground">
            Manage the list of supported banks shown to users.
          </p>
        </div>
        <Button onClick={openCreateDialog} className="gap-2">
          <Plus className="h-4 w-4" />
          Add Bank
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">All Banks</CardTitle>
          <CardDescription>
            {loading ? "Loading..." : `${banks.length} bank${banks.length !== 1 ? "s" : ""} found`}
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {loading ? (
            <div className="p-6 space-y-3">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : banks.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-center">
              <Building2 className="h-12 w-12 text-muted-foreground/40 mb-4" />
              <p className="text-muted-foreground">No banks found.</p>
              <Button variant="outline" onClick={openCreateDialog} className="mt-4 gap-2">
                <Plus className="h-4 w-4" />
                Add your first bank
              </Button>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Logo</TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>Sort Code</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {paginated.map((bank) => (
                  <TableRow key={bank.id}>
                    <TableCell>
                      {bank.logo_url ? (
                        <img
                          src={bank.logo_url}
                          alt={bank.name}
                          className="h-8 w-8 rounded object-contain border bg-white"
                          onError={(e) => {
                            (e.target as HTMLImageElement).style.display = "none";
                          }}
                        />
                      ) : (
                        <div className="h-8 w-8 rounded border bg-muted flex items-center justify-center">
                          <Building2 className="h-4 w-4 text-muted-foreground" />
                        </div>
                      )}
                    </TableCell>
                    <TableCell className="font-medium">{bank.name}</TableCell>
                    <TableCell className="text-muted-foreground font-mono text-sm">
                      {bank.sort_code || "—"}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Switch
                          checked={bank.is_active}
                          onCheckedChange={() => handleToggleActive(bank)}
                        />
                        <Badge variant={bank.is_active ? "success" : "secondary"}>
                          {bank.is_active ? "Active" : "Inactive"}
                        </Badge>
                      </div>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => openEditDialog(bank)}
                        >
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="text-red-500 hover:text-red-700 hover:bg-red-50"
                          onClick={() => {
                            setDeletingId(bank.id);
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
          {!loading && banks.length > pageSize && (
            <DataPagination
              total={banks.length}
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
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{editingBank ? "Edit Bank" : "Add Bank"}</DialogTitle>
            <DialogDescription>
              {editingBank
                ? "Update the bank details."
                : "Add a new bank to the supported list."}
            </DialogDescription>
          </DialogHeader>

          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="name">Bank Name *</Label>
              <Input
                id="name"
                value={formData.name}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                placeholder="Barclays"
                required
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="sort_code">Sort Code</Label>
              <Input
                id="sort_code"
                value={formData.sort_code ?? ""}
                onChange={(e) => setFormData({ ...formData, sort_code: e.target.value })}
                placeholder="20-00-00"
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="logo_url">Logo URL</Label>
              <Input
                id="logo_url"
                value={formData.logo_url ?? ""}
                onChange={(e) => setFormData({ ...formData, logo_url: e.target.value })}
                placeholder="https://example.com/logo.png"
              />
              {formData.logo_url && (
                <img
                  src={formData.logo_url}
                  alt="Preview"
                  className="h-10 w-10 rounded border object-contain bg-white"
                  onError={(e) => ((e.target as HTMLImageElement).style.display = "none")}
                />
              )}
            </div>

            <div className="flex items-center gap-3">
              <Switch
                id="is_active"
                checked={formData.is_active ?? true}
                onCheckedChange={(checked) => setFormData({ ...formData, is_active: checked })}
              />
              <Label htmlFor="is_active">Active</Label>
            </div>

            <DialogFooter className="gap-2">
              <Button type="button" variant="outline" onClick={closeDialog}>
                Cancel
              </Button>
              <Button type="submit" disabled={submitting}>
                {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {editingBank ? "Update Bank" : "Add Bank"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Delete Bank</DialogTitle>
            <DialogDescription>
              Are you sure you want to remove this bank? This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2">
            <Button
              variant="outline"
              onClick={() => { setDeleteDialogOpen(false); setDeletingId(null); }}
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
