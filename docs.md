# API Documentation

**Base URL:** `http://13.60.208.3:8000`

**Auth:** `Authorization: Bearer <token>` for protected routes

## Authentication

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/auth/register` | Public | Register new user |
| POST | `/api/auth/login` | Public | Login with email/phone + password |
| POST | `/api/auth/verify-user` | Public | Check if user exists |
| POST | `/api/auth/refresh` | Public | Refresh access token |
| POST | `/api/auth/google-auth` | Public | Google sign-in via Firebase token |

## User Profile

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/user/profile` | Required | Get own profile |
| POST | `/api/user/update` | Required | Update profile (form data) |
| POST | `/api/user/change-password` | Required | Change password |
| POST | `/api/user/verify-credentials` | Required | Submit identity verification |
| POST | `/api/user/device-token` | Required | Save FCM device token |
| GET | `/api/v1/user/:id/profile` | Public | Get user profile by ID |
| GET | `/api/v1/user/:id/portfolio` | Public | Get user's portfolio |
| GET | `/api/v1/user/:id/certifications` | Public | Get user's certifications |
| POST | `/api/v1/user/:id/portfolio` | Required | Add portfolio item |
| PUT | `/api/v1/user/:id/portfolio` | Required | Update portfolio item |
| DELETE | `/api/v1/user/:id/portfolio` | Required | Delete portfolio item |
| POST | `/api/v1/user/:id/certifications` | Required | Add certification |
| PUT | `/api/v1/user/:id/certifications` | Required | Update certification |
| DELETE | `/api/v1/user/:id/certifications` | Required | Delete certification |

## Jobs

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/job/feed` | Public | Job feed. Query: page, limit, category, query, lat, long |
| GET | `/api/v1/job/:id` | Public | Get job detail |
| POST | `/api/job/create` | Required | Create job post (multipart, up to 6 media files) |
| GET | `/api/job/my` | Required | Get own job posts |
| POST | `/api/job/update/:id` | Required | Update job post |
| GET | `/api/job/:id/contract` | Required | Get contracts for job |
| GET | `/api/job/:id/proposal` | Required | Get proposals for job |

## Proposals & Contracts

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/job/send-proposal` | Required | Submit proposal on a job |
| GET | `/api/job/proposals/my` | Required | Get own proposals |
| GET | `/api/proposals/:id` | Required | Get proposal details |
| PUT | `/api/proposals/:id` | Required | Update proposal |
| POST | `/api/proposals/:id/withdraw` | Required | Withdraw proposal |
| DELETE | `/api/proposals/:id` | Required | Delete proposal |
| POST | `/api/proposals/:id/decision` | Required | Accept/reject proposal |
| GET | `/api/proposals/:id/payment-summary` | Required | Payment breakdown for proposal |
| POST | `/api/proposals/create-contract` | Required | Create contract from proposal |
| GET | `/api/proposals/get-contract` | Required | Get own contracts |
| GET | `/api/contracts/list` | Required | List own contracts |
| POST | `/api/contracts/:id/complete` | Required | Mark contract complete |

## Payments & Escrow

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/v1/webhooks/stripe/payment` | Public | Stripe webhook |
| GET | `/api/v1/payments/transactions` | Required | All transactions. Query: page, limit, user_id, status |
| GET | `/api/v1/payments/transactions/my` | Required | Own transactions |
| GET | `/api/v1/payments/transactions/:id` | Required | Transaction by ID |
| GET | `/api/v1/payments/my` | Required | Own payment details |
| GET | `/api/jobs/:id/payment-details` | Required | Payment breakdown for job |
| POST | `/api/v1/escrow/contracts/:id/deposit` | Required | Initiate escrow deposit → returns client_secret |
| GET | `/api/v1/escrow/contracts/:id/status` | Required | Escrow status |
| POST | `/api/v1/escrow/contracts/:id/refund` | Required | Request refund (client only) |
| GET | `/api/v1/escrow/contracts/:id/summary` | Required | Payment summary with promo discount |

## Wallet & Payouts

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/wallet/balance` | Required | Returns wallet_balance, referral_reward_balance, total_available, currency |
| POST | `/api/v1/wallet/withdraw` | Required | Request withdrawal. Body: `{"amount": <cents>, "currency": "gbp"}` |
| GET | `/api/v1/wallet/withdrawals` | Required | Own withdrawal history |
| GET | `/api/v1/wallet/bank-accounts` | Required | List saved bank accounts |
| POST | `/api/v1/wallet/bank-accounts` | Required | Add bank account (see body below) |
| PUT | `/api/v1/wallet/bank-accounts/:id/default` | Required | Set account as default |
| DELETE | `/api/v1/wallet/bank-accounts/:id` | Required | Delete bank account |

**Add bank account body:**
```json
{
  "account_holder_name": "John Doe",
  "sort_code": "10-88-00",
  "account_number": "00012345",
  "currency": "gbp",
  "set_as_default": true
}
```

## Referrals

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/referrals/validate/:code` | Public | Validate referral code |
| POST | `/api/v1/referrals/codes` | Required | Create referral code |
| GET | `/api/v1/referrals/codes/my` | Required | Own referral codes |
| GET | `/api/v1/referrals/codes/:id` | Required | Get referral code |
| PUT | `/api/v1/referrals/codes/:id` | Required | Update referral code |
| DELETE | `/api/v1/referrals/codes/:id` | Required | Delete referral code |
| GET | `/api/v1/referrals/my` | Required | Own referrals (as referrer) |
| GET | `/api/v1/referrals/status` | Required | Referral status (as referee) |

## Promotions

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/promotions` | Public | List active promotional offers |
| GET | `/api/v1/promotions/my-eligibility` | Required | Check own eligibility. Query: amount |

## Notifications

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/notifications/preferences` | Required | Get notification preferences |
| PUT | `/api/v1/notifications/preferences` | Required | Update preferences (categories, radius, location, toggles) |

## Chat

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET (WS) | `/chat/chat/ws?user_id=` | Public | WebSocket connection |
| GET | `/chat/inbox` | Required | List conversations |
| POST | `/chat/init` | Required | Start chat. Body: {"target_id": "", "job_id": ""} |
| GET | `/chat/:id/history` | Required | Message history. Query: page, limit |
| GET | `/chat/:id/unread` | Required | Unread messages |
| POST | `/chat/:id/read` | Required | Mark as read |
| POST | `/chat/:id/message` | Required | Send message (text + optional file attachments) |

## Categories

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/category/list` | Public | List all active categories |
| GET | `/api/v1/category/:id` | Public | Get category by ID |
| POST | `/api/v1/category/create` | Required | Create category |
| PUT | `/api/v1/category/update/:id` | Required | Update category |
| DELETE | `/api/v1/category/delete/:id` | Required | Delete category |

## Admin

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/admin/users` | Required | List all users. Query: page, limit |
| GET | `/api/v1/admin/users/:id` | Required | Get user details |
| PUT | `/api/v1/admin/users/:id` | Required | Update user (first_name, last_name, email, phone_number, disabled, email_verified, phone_verified, identity_verified) |
| PUT | `/api/v1/admin/users/:id/password` | Required | Change user password. Body: `{"new_password": ""}` |
| GET | `/api/v1/admin/users/:id/wallet` | Required | User wallet + transaction summary |
| GET | `/api/v1/admin/jobs` | Required | List all jobs. Query: page, limit |
| GET | `/api/v1/admin/jobs/:id` | Required | Full job detail (location, proposals, contract, payments) |
| PUT | `/api/v1/admin/jobs/:id` | Required | Update job (title, description, budget, open_budget, address, status, category_id) |
| GET | `/api/v1/admin/payouts` | Required | All withdrawals. Query: page, limit, status, user_id |
| GET | `/api/v1/admin/referrals/codes` | Required | All referral codes |
| GET | `/api/v1/admin/referrals/usages` | Required | All referral usages |
| POST | `/api/v1/admin/promotions` | Required | Create promotional offer |
| GET | `/api/v1/admin/promotions` | Required | List all offers (including inactive) |
| PUT | `/api/v1/admin/promotions/:id` | Required | Update offer |
| DELETE | `/api/v1/admin/promotions/:id` | Required | Delete offer |

**Create/Update promotion body:**
```json
{
  "name": "Welcome Offer",
  "description": "10% off for new users",
  "min_jobs_completed": 0,
  "min_jobs_posted": 0,
  "discount_percentage": 10.0,
  "discount_amount": 0,
  "max_discount_amount": 500,
  "is_active": true,
  "expires_at": "2026-12-31T00:00:00Z"
}
```

## Utility

| Method | Path | Description |
|--------|------|-------------|
| GET | `/ping` | Health check → {"message": "pong"} |

## Common Response Envelope

```json
{
  "message": "success",
  "data": { },
  "error": "optional error string",
  "meta": { "page": 1, "limit": 20, "total": 100 }
}
```
