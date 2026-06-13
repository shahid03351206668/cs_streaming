"use client";

import { useEffect, useState } from "react";
import {
  Plus, Pencil, Trash2, Loader2, Mail, Star, StarOff, FileText, Eye, EyeOff,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from "@/components/ui/card";
import { useToast } from "@/components/ui/use-toast";
import {
  EmailAccount, EmailAccountParams, EmailAccountUpdateParams,
  EmailTemplate, EmailTemplateParams,
  listEmailAccounts, createEmailAccount, updateEmailAccount,
  deleteEmailAccount, setDefaultEmailAccount,
  listEmailTemplates, createEmailTemplate, updateEmailTemplate, deleteEmailTemplate,
} from "@/lib/api";

// ─── Email Accounts ───────────────────────────────────────────────────────────

const defaultAccountForm: EmailAccountParams = {
  name: "", host: "", port: 587, email: "", password: "", from_name: "", is_default: false,
};

function EmailAccountsTab() {
  const { toast } = useToast();
  const [accounts, setAccounts] = useState<EmailAccount[]>([]);
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [editing, setEditing] = useState<EmailAccount | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [form, setForm] = useState<EmailAccountParams>(defaultAccountForm);
  const [updateForm, setUpdateForm] = useState<EmailAccountUpdateParams>({});
  const [showPassword, setShowPassword] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const fetch = async () => {
    try {
      const res = await listEmailAccounts();
      const data = Array.isArray(res.data) ? res.data : (res.data as any)?.data ?? [];
      setAccounts(data);
    } catch {
      toast({ variant: "destructive", title: "Error", description: "Failed to load email accounts." });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetch(); }, []);

  const openCreate = () => {
    setEditing(null);
    setForm(defaultAccountForm);
    setUpdateForm({});
    setShowPassword(false);
    setDialogOpen(true);
  };

  const openEdit = (account: EmailAccount) => {
    setEditing(account);
    setUpdateForm({ name: account.name, host: account.host, port: account.port, from_name: account.from_name, is_active: account.is_active });
    setShowPassword(false);
    setDialogOpen(true);
  };

  const closeDialog = () => {
    setDialogOpen(false);
    setEditing(null);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    try {
      if (editing) {
        await updateEmailAccount(editing.id, updateForm);
        toast({ title: "Updated", description: "Email account updated." });
      } else {
        await createEmailAccount(form);
        toast({ title: "Created", description: "Email account added." });
      }
      closeDialog();
      await fetch();
    } catch {
      toast({ variant: "destructive", title: "Error", description: editing ? "Failed to update." : "Failed to create." });
    } finally {
      setSubmitting(false);
    }
  };

  const handleSetDefault = async (account: EmailAccount) => {
    try {
      await setDefaultEmailAccount(account.id);
      toast({ title: "Default set", description: `${account.name} is now the default account.` });
      await fetch();
    } catch {
      toast({ variant: "destructive", title: "Error", description: "Failed to set default." });
    }
  };

  const handleDelete = async () => {
    if (!deletingId) return;
    try {
      await deleteEmailAccount(deletingId);
      toast({ title: "Deleted", description: "Email account removed." });
      await fetch();
    } catch {
      toast({ variant: "destructive", title: "Error", description: "Failed to delete." });
    } finally {
      setDeleteDialogOpen(false);
      setDeletingId(null);
    }
  };

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0">
          <div>
            <CardTitle className="text-base">Email Accounts</CardTitle>
            <CardDescription>
              {loading ? "Loading..." : `${accounts.length} account${accounts.length !== 1 ? "s" : ""}`}
            </CardDescription>
          </div>
          <Button onClick={openCreate} size="sm" className="gap-2">
            <Plus className="h-4 w-4" /> Add Account
          </Button>
        </CardHeader>
        <CardContent className="p-0">
          {loading ? (
            <div className="p-6 space-y-3">
              {Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} className="h-12 w-full" />)}
            </div>
          ) : accounts.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-center">
              <Mail className="h-12 w-12 text-muted-foreground/40 mb-4" />
              <p className="text-muted-foreground">No email accounts configured.</p>
              <Button variant="outline" onClick={openCreate} className="mt-4 gap-2">
                <Plus className="h-4 w-4" /> Add your first account
              </Button>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Email</TableHead>
                  <TableHead>SMTP Host</TableHead>
                  <TableHead>From Name</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {accounts.map((acc) => (
                  <TableRow key={acc.id}>
                    <TableCell className="font-medium">
                      <div className="flex items-center gap-2">
                        {acc.name}
                        {acc.is_default && (
                          <Badge variant="outline" className="text-xs text-yellow-600 border-yellow-400 bg-yellow-50">
                            Default
                          </Badge>
                        )}
                      </div>
                    </TableCell>
                    <TableCell className="text-muted-foreground font-mono text-sm">{acc.email}</TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {acc.host}:{acc.port}
                    </TableCell>
                    <TableCell className="text-sm">{acc.from_name}</TableCell>
                    <TableCell>
                      <Badge variant={acc.is_active ? "success" : "secondary"}>
                        {acc.is_active ? "Active" : "Inactive"}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        {!acc.is_default && (
                          <Button
                            variant="ghost" size="icon"
                            title="Set as default"
                            onClick={() => handleSetDefault(acc)}
                          >
                            <Star className="h-4 w-4 text-yellow-500" />
                          </Button>
                        )}
                        <Button variant="ghost" size="icon" onClick={() => openEdit(acc)}>
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost" size="icon"
                          className="text-red-500 hover:text-red-700 hover:bg-red-50"
                          onClick={() => { setDeletingId(acc.id); setDeleteDialogOpen(true); }}
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
        </CardContent>
      </Card>

      {/* Create / Edit Dialog */}
      <Dialog open={dialogOpen} onOpenChange={(open) => !open && closeDialog()}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{editing ? "Edit Email Account" : "Add Email Account"}</DialogTitle>
            <DialogDescription>
              {editing ? "Update the SMTP account settings." : "Configure a new outbound email account."}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label>Display Name *</Label>
              <Input
                value={editing ? (updateForm.name ?? "") : form.name}
                onChange={(e) => editing
                  ? setUpdateForm({ ...updateForm, name: e.target.value })
                  : setForm({ ...form, name: e.target.value })}
                placeholder="Tasksy Notifications"
                required={!editing}
              />
            </div>

            {!editing && (
              <div className="space-y-2">
                <Label>Email Address *</Label>
                <Input
                  type="email"
                  value={form.email}
                  onChange={(e) => setForm({ ...form, email: e.target.value })}
                  placeholder="noreply@tasksy.co.uk"
                  required
                />
              </div>
            )}

            <div className="space-y-2">
              <Label>From Name *</Label>
              <Input
                value={editing ? (updateForm.from_name ?? "") : form.from_name}
                onChange={(e) => editing
                  ? setUpdateForm({ ...updateForm, from_name: e.target.value })
                  : setForm({ ...form, from_name: e.target.value })}
                placeholder="Tasksy"
                required={!editing}
              />
            </div>

            <div className="grid grid-cols-3 gap-3">
              <div className="col-span-2 space-y-2">
                <Label>SMTP Host *</Label>
                <Input
                  value={editing ? (updateForm.host ?? "") : form.host}
                  onChange={(e) => editing
                    ? setUpdateForm({ ...updateForm, host: e.target.value })
                    : setForm({ ...form, host: e.target.value })}
                  placeholder="smtp.example.com"
                  required={!editing}
                />
              </div>
              <div className="space-y-2">
                <Label>Port *</Label>
                <Input
                  type="number"
                  value={editing ? (updateForm.port ?? 587) : form.port}
                  onChange={(e) => editing
                    ? setUpdateForm({ ...updateForm, port: Number(e.target.value) })
                    : setForm({ ...form, port: Number(e.target.value) })}
                  required={!editing}
                />
              </div>
            </div>

            <div className="space-y-2">
              <Label>{editing ? "New Password (leave blank to keep)" : "SMTP Password *"}</Label>
              <div className="relative">
                <Input
                  type={showPassword ? "text" : "password"}
                  value={editing ? (updateForm.password ?? "") : form.password}
                  onChange={(e) => editing
                    ? setUpdateForm({ ...updateForm, password: e.target.value })
                    : setForm({ ...form, password: e.target.value })}
                  placeholder={editing ? "Leave blank to keep current" : "••••••••"}
                  required={!editing}
                  className="pr-10"
                />
                <button
                  type="button"
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                  onClick={() => setShowPassword(!showPassword)}
                >
                  {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
            </div>

            {editing && (
              <div className="flex items-center gap-3">
                <Switch
                  checked={updateForm.is_active ?? editing.is_active}
                  onCheckedChange={(v) => setUpdateForm({ ...updateForm, is_active: v })}
                />
                <Label>Active</Label>
              </div>
            )}

            {!editing && (
              <div className="flex items-center gap-3">
                <Switch
                  checked={form.is_default ?? false}
                  onCheckedChange={(v) => setForm({ ...form, is_default: v })}
                />
                <Label>Set as default</Label>
              </div>
            )}

            <DialogFooter className="gap-2">
              <Button type="button" variant="outline" onClick={closeDialog}>Cancel</Button>
              <Button type="submit" disabled={submitting}>
                {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {editing ? "Update Account" : "Add Account"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete Confirm */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Delete Email Account</DialogTitle>
            <DialogDescription>
              Are you sure? This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2">
            <Button variant="outline" onClick={() => { setDeleteDialogOpen(false); setDeletingId(null); }}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDelete}>Delete</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

// ─── Email Templates ──────────────────────────────────────────────────────────

const defaultTemplateForm: EmailTemplateParams = { name: "", subject: "", body: "" };

const TEMPLATE_HINTS: Record<string, string> = {
  proposal_received: "Sent to client when a freelancer submits a proposal.",
  proposal_accepted: "Sent to freelancer when their proposal is accepted.",
  proposal_rejected: "Sent to freelancer when their proposal is rejected.",
  payment_released: "Sent to freelancer when escrow funds are released.",
  payout_completed: "Sent to freelancer when a payout is confirmed by Stripe.",
  payout_failed: "Sent to freelancer when a payout fails.",
  job_completed_client: "Sent to client when the job is fully completed.",
  job_completed_freelancer: "Sent to freelancer when the job is fully completed.",
};

const VARIABLES_BY_TEMPLATE: Record<string, string[]> = {
  proposal_received: ["{{first_name}}", "{{job_title}}", "{{freelancer_name}}"],
  proposal_accepted: ["{{first_name}}", "{{job_title}}"],
  proposal_rejected: ["{{first_name}}", "{{job_title}}"],
  payment_released: ["{{first_name}}", "{{job_title}}"],
  payout_completed: ["{{first_name}}", "{{amount}}", "{{currency}}"],
  payout_failed: ["{{first_name}}", "{{amount}}", "{{currency}}", "{{failure_reason}}"],
  job_completed_client: ["{{first_name}}", "{{job_title}}", "{{freelancer_name}}"],
  job_completed_freelancer: ["{{first_name}}", "{{job_title}}", "{{client_name}}"],
};

function EmailTemplatesTab() {
  const { toast } = useToast();
  const [templates, setTemplates] = useState<EmailTemplate[]>([]);
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [editing, setEditing] = useState<EmailTemplate | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [form, setForm] = useState<EmailTemplateParams>(defaultTemplateForm);
  const [submitting, setSubmitting] = useState(false);

  const fetch = async () => {
    try {
      const res = await listEmailTemplates();
      const data = Array.isArray(res.data) ? res.data : (res.data as any)?.data ?? [];
      setTemplates(data);
    } catch {
      toast({ variant: "destructive", title: "Error", description: "Failed to load templates." });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetch(); }, []);

  const openCreate = () => {
    setEditing(null);
    setForm(defaultTemplateForm);
    setDialogOpen(true);
  };

  const openEdit = (t: EmailTemplate) => {
    setEditing(t);
    setForm({ name: t.name, subject: t.subject, body: t.body });
    setDialogOpen(true);
  };

  const closeDialog = () => { setDialogOpen(false); setEditing(null); };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    try {
      if (editing) {
        await updateEmailTemplate(editing.id, { subject: form.subject, body: form.body });
        toast({ title: "Updated", description: "Template updated." });
      } else {
        await createEmailTemplate(form);
        toast({ title: "Created", description: "Template created." });
      }
      closeDialog();
      await fetch();
    } catch {
      toast({ variant: "destructive", title: "Error", description: editing ? "Failed to update." : "Failed to create." });
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (!deletingId) return;
    try {
      await deleteEmailTemplate(deletingId);
      toast({ title: "Deleted", description: "Template removed." });
      await fetch();
    } catch {
      toast({ variant: "destructive", title: "Error", description: "Failed to delete." });
    } finally {
      setDeleteDialogOpen(false);
      setDeletingId(null);
    }
  };

  const insertVar = (v: string) => {
    setForm((f) => ({ ...f, body: f.body + v }));
  };

  const vars = VARIABLES_BY_TEMPLATE[form.name] ?? [];
  const hint = TEMPLATE_HINTS[form.name];

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0">
          <div>
            <CardTitle className="text-base">Email Templates</CardTitle>
            <CardDescription>
              {loading ? "Loading..." : `${templates.length} template${templates.length !== 1 ? "s" : ""}`}
            </CardDescription>
          </div>
          <Button onClick={openCreate} size="sm" className="gap-2">
            <Plus className="h-4 w-4" /> Add Template
          </Button>
        </CardHeader>
        <CardContent className="p-0">
          {loading ? (
            <div className="p-6 space-y-3">
              {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-12 w-full" />)}
            </div>
          ) : templates.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-center">
              <FileText className="h-12 w-12 text-muted-foreground/40 mb-4" />
              <p className="text-muted-foreground">No email templates yet.</p>
              <Button variant="outline" onClick={openCreate} className="mt-4 gap-2">
                <Plus className="h-4 w-4" /> Create your first template
              </Button>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Subject</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {templates.map((t) => (
                  <TableRow key={t.id}>
                    <TableCell>
                      <div>
                        <p className="font-medium font-mono text-sm">{t.name}</p>
                        {TEMPLATE_HINTS[t.name] && (
                          <p className="text-xs text-muted-foreground mt-0.5">{TEMPLATE_HINTS[t.name]}</p>
                        )}
                      </div>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground max-w-xs truncate">
                      {t.subject}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        <Button variant="ghost" size="icon" onClick={() => openEdit(t)}>
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost" size="icon"
                          className="text-red-500 hover:text-red-700 hover:bg-red-50"
                          onClick={() => { setDeletingId(t.id); setDeleteDialogOpen(true); }}
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
        </CardContent>
      </Card>

      {/* Create / Edit Dialog */}
      <Dialog open={dialogOpen} onOpenChange={(open) => !open && closeDialog()}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{editing ? "Edit Template" : "Create Template"}</DialogTitle>
            <DialogDescription>
              Use <code className="text-xs bg-muted px-1 py-0.5 rounded">{"{{variable}}"}</code> placeholders in the subject and body.
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label>Template Name *</Label>
              <Input
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder="proposal_received"
                disabled={!!editing}
                required
              />
              {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
            </div>

            <div className="space-y-2">
              <Label>Subject *</Label>
              <Input
                value={form.subject}
                onChange={(e) => setForm({ ...form, subject: e.target.value })}
                placeholder="You have a new proposal for {{job_title}}"
                required
              />
            </div>

            {vars.length > 0 && (
              <div className="space-y-1.5">
                <p className="text-xs font-medium text-muted-foreground">Available variables — click to insert into body:</p>
                <div className="flex flex-wrap gap-1.5">
                  {vars.map((v) => (
                    <button
                      key={v}
                      type="button"
                      onClick={() => insertVar(v)}
                      className="text-xs px-2 py-0.5 rounded border border-dashed border-blue-400 text-blue-600 bg-blue-50 hover:bg-blue-100 font-mono"
                    >
                      {v}
                    </button>
                  ))}
                </div>
              </div>
            )}

            <div className="space-y-2">
              <Label>HTML Body *</Label>
              <textarea
                value={form.body}
                onChange={(e) => setForm({ ...form, body: e.target.value })}
                rows={12}
                required
                placeholder="<p>Hi {{first_name}},</p>..."
                className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm font-mono shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring resize-y"
              />
            </div>

            <DialogFooter className="gap-2">
              <Button type="button" variant="outline" onClick={closeDialog}>Cancel</Button>
              <Button type="submit" disabled={submitting}>
                {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {editing ? "Update Template" : "Create Template"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete Confirm */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Delete Template</DialogTitle>
            <DialogDescription>
              This template will be removed. Emails using this name will stop sending.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2">
            <Button variant="outline" onClick={() => { setDeleteDialogOpen(false); setDeletingId(null); }}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDelete}>Delete</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function EmailPage() {
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold tracking-tight">Email</h2>
        <p className="text-muted-foreground">
          Manage outbound SMTP accounts and transactional email templates.
        </p>
      </div>

      <Tabs defaultValue="accounts">
        <TabsList>
          <TabsTrigger value="accounts" className="gap-2">
            <Mail className="h-4 w-4" /> Accounts
          </TabsTrigger>
          <TabsTrigger value="templates" className="gap-2">
            <FileText className="h-4 w-4" /> Templates
          </TabsTrigger>
        </TabsList>

        <TabsContent value="accounts" className="mt-4">
          <EmailAccountsTab />
        </TabsContent>

        <TabsContent value="templates" className="mt-4">
          <EmailTemplatesTab />
        </TabsContent>
      </Tabs>
    </div>
  );
}
