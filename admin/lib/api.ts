import axios from "axios";

// Server-side (SSR/RSC): call backend directly via env var.
// Client-side (browser): use relative URLs — Next.js rewrites proxy /api/* to the backend.
const API_BASE_URL = "http://13.60.208.3:8000"
// const API_BASE_URL = "http://localhost:5679";

const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    "Content-Type": "application/json",
  },
});

api.interceptors.request.use(
  (config) => {
    if (typeof window !== "undefined") {
      const token = localStorage.getItem("tasksy_token");
      if (token) {
        config.headers.Authorization = `Bearer ${token}`;
      }
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      if (typeof window !== "undefined") {
        localStorage.removeItem("tasksy_token");
        window.location.href = "/login";
      }
    }
    return Promise.reject(error);
  }
);

export default api;

// Auth
export const login = (email: string, password: string) =>
  api.post<{ token: string }>("/api/auth/login", { email, password });

// Users (admin)
export const getUsers = () => api.get("/api/v1/admin/users");
export const getUserById = (id: string) => api.get(`/api/v1/admin/users/${id}`);

export interface AdminUpdateUserParams {
  first_name?: string;
  last_name?: string;
  email?: string;
  phone_number?: string;
  disabled?: boolean;
  email_verified?: boolean;
  phone_verified?: boolean;
  identity_verified?: boolean;
}

export const adminUpdateUser = (id: string, data: AdminUpdateUserParams) =>
  api.put(`/api/v1/admin/users/${id}`, data);

export const adminChangeUserPassword = (id: string, new_password: string) =>
  api.put(`/api/v1/admin/users/${id}/password`, { new_password });

// Jobs (admin)
export const getJobs = (limit = 100, page = 0) =>
  api.get(`/api/v1/admin/jobs?limit=${limit}&page=${page}`);

export const getJobDetail = (id: string) =>
  api.get(`/api/v1/admin/jobs/${id}`);

export interface AdminUpdateJobParams {
  title?: string;
  description?: string;
  budget?: number;
  open_budget?: boolean;
  address?: string;
  status?: string;
  category_id?: string;
}

export const adminUpdateJob = (id: string, data: AdminUpdateJobParams) =>
  api.put(`/api/v1/admin/jobs/${id}`, data);

// Promotions
export interface CreateOfferParams {
  name: string;
  description: string;
  min_jobs_completed: number;
  min_jobs_posted: number;
  discount_percentage: number;
  discount_amount: number;
  max_discount_amount: number;
  is_active: boolean;
  expires_at?: string;
}

export const getPromotions = () =>
  api.get("/api/v1/admin/promotions");

export const createPromotion = (data: CreateOfferParams) =>
  api.post("/api/v1/admin/promotions", data);

export const updatePromotion = (id: string, data: Partial<CreateOfferParams>) =>
  api.put(`/api/v1/admin/promotions/${id}`, data);

export const deletePromotion = (id: string) =>
  api.delete(`/api/v1/admin/promotions/${id}`);

// Referrals
export const getReferralCodes = () =>
  api.get("/api/v1/admin/referrals/codes");

export const getReferralUsages = () =>
  api.get("/api/v1/admin/referrals/usages");

// Transactions
export interface TransactionUser {
  id: string;
  first_name: string;
  last_name: string;
  email: string;
  profile_photo?: string;
}

export interface PaymentTransaction {
  id: string;
  transaction_date: string;
  from_user: TransactionUser;
  to_user: TransactionUser;
  amount: number;
  app_fee_amount: number;
  discount_amount: number;
  net_amount: number;
  currency: string;
  status: string;
  payment_method?: string;
  reference_type: string;
  reference_id: string;
  referral_code_id?: string;
  created_at: string;
}

export interface TransactionFilters {
  page?: number;
  limit?: number;
  status?: string;
  user_id?: string;
  from_date?: string;
  to_date?: string;
  search?: string;
  reference_type?: string;
}

export interface PaginatedMeta {
  total: number;
  page: number;
  limit: number;
  total_pages: number;
}

export const getTransactions = (filters: TransactionFilters = {}) => {
  const params = new URLSearchParams();
  if (filters.page)           params.set("page",           String(filters.page));
  if (filters.limit)          params.set("limit",          String(filters.limit));
  if (filters.status)         params.set("status",         filters.status);
  if (filters.user_id)        params.set("user_id",        filters.user_id);
  if (filters.from_date)      params.set("from_date",      filters.from_date);
  if (filters.to_date)        params.set("to_date",        filters.to_date);
  if (filters.search)         params.set("search",         filters.search);
  if (filters.reference_type) params.set("reference_type", filters.reference_type);
  return api.get<{ message: string; data: PaymentTransaction[]; meta: PaginatedMeta }>(
    `/api/v1/payments/transactions?${params.toString()}`
  );
};

export const getTransactionById = (id: string) =>
  api.get<{ message: string; data: PaymentTransaction }>(`/api/v1/payments/transactions/${id}`);

// Transactions V3 (Stripe Connect Express escrow flow)
export interface PaymentTransactionV3 {
  id: string;
  created_at: string;
  stripe_payment_intent_id: string;
  stripe_charge_id: string;
  from_user_id: string;
  to_user_id: string;
  from_user: TransactionUser;
  to_user: TransactionUser;
  contract_id: string;
  // shopspring/decimal serializes as a quoted JSON string, not a number — parseFloat before use.
  gross_amount: string;
  platform_fee: string;
  net_amount: string;
  currency: string;
  status: string; // held | released | refunded
  flow_version: string;
}

export interface TransactionV3Filters {
  page?: number;
  limit?: number;
  status?: string;
  from_date?: string;
  to_date?: string;
  search?: string;
}

export const getTransactionsV3 = (filters: TransactionV3Filters = {}) => {
  const params = new URLSearchParams();
  if (filters.page)      params.set("page",      String(filters.page));
  if (filters.limit)     params.set("limit",     String(filters.limit));
  if (filters.status)    params.set("status",    filters.status);
  if (filters.from_date) params.set("from_date", filters.from_date);
  if (filters.to_date)   params.set("to_date",   filters.to_date);
  if (filters.search)    params.set("search",    filters.search);
  return api.get<{ message: string; data: PaymentTransactionV3[]; meta: PaginatedMeta }>(
    `/api/v3/admin/payments/transactions?${params.toString()}`
  );
};

export interface PaymentTransactionV3Detail {
  transaction: PaymentTransactionV3;
  escrow: {
    id: string;
    amount: string;
    currency: string;
    status: string;
    stripe_transfer_id: string;
    held_at: string;
    released_at: string | null;
  };
  contract: {
    id: string;
    title: string;
    status: string;
    total_amount: number;
    client_completed: boolean;
    freelancer_completed: boolean;
    start_date: string;
    end_date: string;
  };
  job_post: {
    id: string;
    title: string;
    status: string;
    budget: number;
    open_budget: boolean;
    posted_at: string;
  };
  proposal: {
    id: string;
    status: string;
    bid_amount: number;
    duration: number;
  };
  total_proposals: number;
}

export const getTransactionV3ById = (id: string) =>
  api.get<{ message: string; data: PaymentTransactionV3Detail }>(
    `/api/v3/admin/payments/transactions/${id}`
  );

export interface PaymentV3DailyStat {
  date: string;
  gross_volume: number;
  platform_fee: number;
  count: number;
}

export interface PaymentV3StatusStat {
  status: string;
  count: number;
}

export interface PaymentV3Stats {
  daily: PaymentV3DailyStat[];
  status_breakdown: PaymentV3StatusStat[];
  summary: {
    total_volume: number;
    total_fees: number;
    total_net: number;
  };
}

export const getPaymentV3Stats = (days = 30) =>
  api.get<{ message: string; data: PaymentV3Stats }>(
    `/api/v3/admin/payments/stats?days=${days}`
  );

// Categories
export const getCategories = () =>
  api.get("/api/v1/category/list");

export const createCategory = (data: { name: string }) =>
  api.post("/api/v1/category/create", data);

export const updateCategory = (id: string, data: { name: string }) =>
  api.put(`/api/v1/category/update/${id}`, data);

export const deleteCategory = (id: string) =>
  api.delete(`/api/v1/category/delete/${id}`);

export const reorderCategories = (ids: string[]) =>
  api.put("/api/v1/category/reorder", { ids });

// Escrow
export const depositEscrow = (contractId: string) =>
  api.post(`/api/v1/escrow/contracts/${contractId}/deposit`);

export const getEscrowStatus = (contractId: string) =>
  api.get(`/api/v1/escrow/contracts/${contractId}/status`);

export const refundEscrow = (contractId: string) =>
  api.post(`/api/v1/escrow/contracts/${contractId}/refund`);

// System Settings
export interface SystemSettings {
  id: string;
  client_commission_percentage: number;
  freelancer_commission_percentage: number;
  application_fee_amount: number;
  app_fee_percentage: number;
  referral_discount_percentage: number;
  referral_reward_amount: number;
}

export const getSystemSettings = () =>
  api.get<{ data: SystemSettings }>("/api/v1/admin/settings");

export const updateSystemSettings = (data: Partial<SystemSettings>) =>
  api.put<{ data: SystemSettings }>("/api/v1/admin/settings", data);

// Banks
export interface Bank {
  id: string;
  name: string;
  sort_code: string;
  logo_url: string;
  is_active: boolean;
  created_at: string;
}

export interface BankParams {
  name: string;
  sort_code?: string;
  logo_url?: string;
  is_active?: boolean;
}

export const listBanks = () =>
  api.get<{ data: Bank[] }>("/api/v1/banks");

export const adminListBanks = () =>
  api.get<{ data: Bank[] }>("/api/v1/admin/banks");

export const adminCreateBank = (data: BankParams) =>
  api.post<{ data: Bank }>("/api/v1/admin/banks", data);

export const adminUpdateBank = (id: string, data: Partial<BankParams>) =>
  api.put<{ data: Bank }>(`/api/v1/admin/banks/${id}`, data);

export const adminDeleteBank = (id: string) =>
  api.delete(`/api/v1/admin/banks/${id}`);

// Ledger
export interface LedgerAccount {
  id: string;
  name: string;
  type: string;
  user_id?: string;
}

export interface LedgerGLEntry {
  id: string;
  account_id: string;
  account: LedgerAccount;
  amount: number;
  category: string;
  created_at: string;
}

export interface LedgerTransaction {
  id: string;
  type: string;
  reference_id: string;
  status: string;
  description: string;
  posting_date: string;
  created_at: string;
  entries: LedgerGLEntry[];
}

export interface LedgerSummary {
  total_escrow: number;
  total_revenue: number;
  total_marketing: number;
}

export interface LedgerReportResponse {
  system_balance: number;
  system_healthy: boolean;
  summary: LedgerSummary;
  transactions: LedgerTransaction[];
  meta: { total: number; page: number; limit: number; total_pages: number };
}

export const getLedgerReport = (params?: {
  page?: number;
  limit?: number;
  type?: string;
  from_date?: string;
  to_date?: string;
  search?: string;
}) => {
  const query = new URLSearchParams();
  if (params?.page)      query.set("page",      String(params.page));
  if (params?.limit)     query.set("limit",     String(params.limit));
  if (params?.type)      query.set("type",      params.type);
  if (params?.from_date) query.set("from_date", params.from_date);
  if (params?.to_date)   query.set("to_date",   params.to_date);
  if (params?.search)    query.set("search",    params.search);
  return api.get<{ message: string; data: LedgerReportResponse }>(
    `/api/v1/admin/ledger?${query.toString()}`
  );
};

// Disputes
export type DisputeStatus = "open" | "resolved" | "closed";

export interface DisputeUser {
  id: string;
  first_name: string;
  last_name: string;
  email: string;
  profile_photo?: string;
}

export interface DisputeContract {
  id: string;
  title: string;
  status: string;
  client_id: string;
  freelancer_id: string;
}

export interface DisputeAttachment {
  id: string;
  url: string;
  file_name: string;
  file_size: number;
  media_type: string;
  object_key: string;
  entity_id: string;
  entity_type: string;
  created_at: string;
}

export interface Dispute {
  id: string;
  contract_id: string;
  contract: DisputeContract;
  filed_by_id: string;
  filed_by: DisputeUser;
  /** "client" or "freelancer" — role of the filer on the contract */
  filed_by_role: "client" | "freelancer";
  reason: string;
  description: string;
  status: DisputeStatus;
  resolved_by_id?: string;
  resolved_by?: DisputeUser;
  resolution?: string;
  resolved_at?: string;
  attachments?: DisputeAttachment[];
  created_at: string;
  updated_at: string;
}

export interface DisputeListMeta {
  page: number;
  limit: number;
  total: number;
  total_pages: number;
}

export const adminListDisputes = (params: {
  page?: number;
  limit?: number;
  status?: string;
  contract_id?: string;
}) => {
  const query = new URLSearchParams();
  if (params.page) query.set("page", String(params.page));
  if (params.limit) query.set("limit", String(params.limit));
  if (params.status) query.set("status", params.status);
  if (params.contract_id) query.set("contract_id", params.contract_id);
  return api.get<{ message: string; data: Dispute[]; meta: DisputeListMeta }>(
    `/api/v1/admin/disputes?${query.toString()}`
  );
};

export const adminGetDispute = (id: string) =>
  api.get<{ message: string; data: Dispute }>(`/api/v1/admin/disputes/${id}`);

export const adminResolveDispute = (id: string, resolution: string) =>
  api.put<{ message: string; data: Dispute }>(`/api/v1/admin/disputes/${id}/resolve`, { resolution });

// Email Accounts
export interface EmailAccount {
  id: string;
  name: string;
  host: string;
  port: number;
  email: string;
  from_name: string;
  is_active: boolean;
  is_default: boolean;
}

export interface EmailAccountParams {
  name: string;
  host: string;
  port: number;
  email: string;
  password: string;
  from_name: string;
  is_default?: boolean;
}

export interface EmailAccountUpdateParams {
  name?: string;
  host?: string;
  port?: number;
  username?: string;
  password?: string;
  from_name?: string;
  is_active?: boolean;
}

export const listEmailAccounts = () =>
  api.get<EmailAccount[]>("/api/v1/admin/email-accounts");

export const createEmailAccount = (data: EmailAccountParams) =>
  api.post<EmailAccount>("/api/v1/admin/email-accounts", data);

export const updateEmailAccount = (id: string, data: EmailAccountUpdateParams) =>
  api.put("/api/v1/admin/email-accounts/" + id, data);

export const deleteEmailAccount = (id: string) =>
  api.delete("/api/v1/admin/email-accounts/" + id);

export const setDefaultEmailAccount = (id: string) =>
  api.put("/api/v1/admin/email-accounts/" + id + "/default");

// Email Templates
export interface EmailTemplate {
  id: string;
  name: string;
  subject: string;
  body: string;
}

export interface EmailTemplateParams {
  name: string;
  subject: string;
  body: string;
}

export const listEmailTemplates = () =>
  api.get<EmailTemplate[]>("/api/v1/admin/email-templates");

export const createEmailTemplate = (data: EmailTemplateParams) =>
  api.post<EmailTemplate>("/api/v1/admin/email-templates", data);

export const updateEmailTemplate = (id: string, data: Partial<EmailTemplateParams>) =>
  api.put("/api/v1/admin/email-templates/" + id, data);

export const deleteEmailTemplate = (id: string) =>
  api.delete("/api/v1/admin/email-templates/" + id);
