import axios from "axios";

// Server-side (SSR/RSC): call backend directly via env var.
// Client-side (browser): use relative URLs — Next.js rewrites proxy /api/* to the backend.
const API_BASE_URL = typeof window === 'undefined'
  ? (process.env.BACKEND_URL || 'http://localhost:5000')
  : '';

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
export interface PaymentTransaction {
  id: string;
  from_user_id: string;
  to_user_id: string;
  amount: number;
  currency: string;
  status: string;
  payment_method: string;
  reference_type: string;
  reference_id: string;
  app_fee_amount: number;
  net_amount: number;
  discount_amount: number;
  created_at: string;
}

export const getTransactions = () =>
  api.get<PaymentTransaction[]>("/api/v1/payments/transactions");

// Categories
export const getCategories = () =>
  api.get("/api/v1/category/list");

export const createCategory = (data: { name: string }) =>
  api.post("/api/v1/category/create", data);

export const updateCategory = (id: string, data: { name: string }) =>
  api.put(`/api/v1/category/update/${id}`, data);

export const deleteCategory = (id: string) =>
  api.delete(`/api/v1/category/delete/${id}`);

// Escrow
export const depositEscrow = (contractId: string) =>
  api.post(`/api/v1/escrow/contracts/${contractId}/deposit`);

export const getEscrowStatus = (contractId: string) =>
  api.get(`/api/v1/escrow/contracts/${contractId}/status`);

export const refundEscrow = (contractId: string) =>
  api.post(`/api/v1/escrow/contracts/${contractId}/refund`);
