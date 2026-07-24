Customer
    │
    ▼
Create Payment Intent / Checkout Session
    │
    ▼
Customer Pays
    │
    ▼
Payment Succeeded (Webhook)
    │
    ▼
Verify Webhook Signature
    │
    ▼
Create Transaction Record
    │
    ▼
Hold Funds in Escrow
    │
    ▼
Calculate Amounts
    ├── Gross Amount
    ├── Platform Commission #1
    ├── Platform Commission #2
    ├── Application Fee
    └── Net Escrow Amount
    │
    ▼
Update Wallets & Ledger
    ├── Customer Wallet
    ├── Provider Wallet (Pending)
    └── Platform Revenue
    │
    ▼
Job In Progress
    │
    ├── If Cancelled
    │      ├── Refund (Full/Partial)
    │      └── Reverse Pending Balances
    │
    ▼
Job Completed
    │
    ▼
Release Escrow
    │
    ▼
Transfer Net Amount to Provider Wallet
    │
    ▼
(Optional) Instant/Bank Payout
    │
    ▼
Mark Transaction as Settled
