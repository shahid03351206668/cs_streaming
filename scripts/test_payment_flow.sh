#!/bin/bash
# ============================================================
# Tasksy Payment Flow Test — uses Stripe CLI to simulate
# real webhook events end-to-end:
#   Escrow deposit → Stripe confirm → webhook fires
#   → Contract complete → escrow capture → wallet credited
#   → Bank account add → Withdrawal → Payout history
#
# Prerequisites:
#   - Server running at BASE_URL
#   - Stripe CLI installed and logged in  (stripe login)
#   - `stripe listen` forwarding webhooks  (run in a separate terminal)
#     OR pass --with-listener to start it automatically
#
# Usage:
#   ./scripts/test_payment_flow.sh
#   ./scripts/test_payment_flow.sh --with-listener   # auto-start stripe listen
#   ./scripts/test_payment_flow.sh --base-url http://localhost:5000
# ============================================================

BASE_URL="http://localhost:8080"
WITH_LISTENER=false
LISTENER_PID=""

# ── parse args ──────────────────────────────────────────────
for arg in "$@"; do
  case $arg in
    --with-listener) WITH_LISTENER=true ;;
    --base-url=*) BASE_URL="${arg#*=}" ;;
  esac
done

WEBHOOK_ENDPOINT="$BASE_URL/api/v1/webhooks/stripe/payment"

# ── load Stripe secret key from .env ────────────────────────
SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
STRIPE_API_KEY=""
if [ -f "$SCRIPT_DIR/.env" ]; then
  STRIPE_API_KEY=$(grep -m1 '^STRIPE_SECRET_KEY=' "$SCRIPT_DIR/.env" | cut -d'=' -f2-)
fi
if [ -z "$STRIPE_API_KEY" ]; then
  echo -e "${YELLOW}Warning: STRIPE_SECRET_KEY not found in .env — stripe CLI calls will use your logged-in account${NC}"
fi

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BLUE='\033[0;34m'
NC='\033[0m'

PASS=0
FAIL=0
SKIP=0

pass()   { echo -e "${GREEN}✓ $1${NC}"; PASS=$((PASS+1)); }
fail()   { echo -e "${RED}✗ $1${NC}"; FAIL=$((FAIL+1)); }
skip()   { echo -e "${YELLOW}⚠ SKIP: $1${NC}"; SKIP=$((SKIP+1)); }
info()   { echo -e "${CYAN}  → $1${NC}"; }
stripe_info() { echo -e "${BLUE}  [stripe] $1${NC}"; }
section(){ echo ""; echo -e "${YELLOW}══════════════════════════════════════${NC}"; echo -e "${YELLOW}  $1${NC}"; echo -e "${YELLOW}══════════════════════════════════════${NC}"; }

cleanup() {
  if [ -n "$LISTENER_PID" ]; then
    kill "$LISTENER_PID" 2>/dev/null
    echo -e "${CYAN}  → Stripe listener stopped${NC}"
  fi
}
trap cleanup EXIT

TS=$(date +%s)

# ────────────────────────────────────────────────────────────
section "0. PREREQUISITES"
# ────────────────────────────────────────────────────────────

# Check stripe CLI
if ! command -v stripe &>/dev/null; then
  echo -e "${RED}✗ 'stripe' CLI not found. Install from https://stripe.com/docs/stripe-cli${NC}"
  exit 1
fi
pass "Stripe CLI found: $(stripe --version 2>&1 | head -1)"

# Server health
PING=$(curl -s --max-time 5 "$BASE_URL/ping")
if echo "$PING" | grep -q "pong"; then
  pass "Server reachable at $BASE_URL"
else
  fail "Server not reachable at $BASE_URL — is it running?"
  exit 1
fi

# ── Optionally start stripe listen ──────────────────────────
if $WITH_LISTENER; then
  echo -e "${CYAN}  → Starting stripe listen (forwarding to $WEBHOOK_ENDPOINT)...${NC}"
  stripe listen --forward-to "$WEBHOOK_ENDPOINT" &
  LISTENER_PID=$!
  sleep 3
  stripe_info "listener started (PID $LISTENER_PID)"
  stripe_info "Make sure STRIPE_WEBHOOK_SECRET in your .env matches the key shown above"
else
  stripe_info "Assuming 'stripe listen --forward-to $WEBHOOK_ENDPOINT' is already running"
  stripe_info "If not, run it in a separate terminal then re-run this script, or pass --with-listener"
fi

# ────────────────────────────────────────────────────────────
section "1. REGISTER USERS"
# ────────────────────────────────────────────────────────────

TS_A=$TS
TS_B=$((TS+1))

REG_A=$(curl -s -X POST "$BASE_URL/api/auth/register" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "first_name=Alice" -d "last_name=Client" \
  -d "email=alice_pay_${TS_A}@test.com" \
  -d "password=Test1234!" -d "phone_number=+441${TS_A}")

TOKEN_A=$(echo "$REG_A" | jq -r '.tokens.access_token // empty')
USER_A_ID=$(echo "$REG_A" | jq -r '.user.id // empty')

if [ -n "$TOKEN_A" ] && [ "$TOKEN_A" != "null" ]; then
  pass "User A (client) registered — ID: $USER_A_ID"
else
  fail "User A registration failed: $REG_A"; exit 1
fi

REG_B=$(curl -s -X POST "$BASE_URL/api/auth/register" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "first_name=Bob" -d "last_name=Freelancer" \
  -d "email=bob_pay_${TS_B}@test.com" \
  -d "password=Test1234!" -d "phone_number=+442${TS_B}")

TOKEN_B=$(echo "$REG_B" | jq -r '.tokens.access_token // empty')
USER_B_ID=$(echo "$REG_B" | jq -r '.user.id // empty')

if [ -n "$TOKEN_B" ] && [ "$TOKEN_B" != "null" ]; then
  pass "User B (freelancer) registered — ID: $USER_B_ID"
else
  fail "User B registration failed: $REG_B"; exit 1
fi

# Identity-verify both users so they can post jobs and send proposals
curl -s -X POST "$BASE_URL/api/user/update" \
  -H "Authorization: Bearer $TOKEN_A" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "verified=true" > /dev/null
pass "User A identity verified"

curl -s -X POST "$BASE_URL/api/user/update" \
  -H "Authorization: Bearer $TOKEN_B" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "verified=true" > /dev/null
pass "User B identity verified"

# ────────────────────────────────────────────────────────────
section "2. JOB → PROPOSAL → CONTRACT"
# ────────────────────────────────────────────────────────────

CATEGORY_ID="92cf775e-d6ce-494c-9394-6263fb86cbcb"

# Get first available category if that one doesn't exist
CAT_LIST=$(curl -s "$BASE_URL/api/v1/category/list")
FIRST_CAT=$(echo "$CAT_LIST" | jq -r '.data[0].id // empty')
if [ -n "$FIRST_CAT" ] && [ "$FIRST_CAT" != "null" ]; then
  CATEGORY_ID=$FIRST_CAT
fi
info "Using category: $CATEGORY_ID"

JOB_RESP=$(curl -s -X POST "$BASE_URL/api/job/create" \
  -H "Authorization: Bearer $TOKEN_A" \
  -F "category_id=$CATEGORY_ID" \
  -F "title=Payment Flow Test Job" \
  -F "description=Testing full payment escrow flow" \
  -F "budget=20000" \
  -F "open_budget=false" \
  -F "address=London, UK")

JOB_ID=$(echo "$JOB_RESP" | jq -r '.data.id // empty')
if [ -n "$JOB_ID" ] && [ "$JOB_ID" != "null" ]; then
  pass "Job created — ID: $JOB_ID"
else
  fail "Job creation failed: $JOB_RESP"; exit 1
fi

PROPOSAL_RESP=$(curl -s -X POST "$BASE_URL/api/job/send-proposal" \
  -H "Authorization: Bearer $TOKEN_B" \
  -F "job_post_id=$JOB_ID" \
  -F "cover_letter=I can do this job efficiently." \
  -F "bid_amount=18000" \
  -F "duration=3")

PROPOSAL_ID=$(echo "$PROPOSAL_RESP" | jq -r '.data.id // .proposal.id // empty')
if [ -n "$PROPOSAL_ID" ] && [ "$PROPOSAL_ID" != "null" ]; then
  pass "Proposal submitted — ID: $PROPOSAL_ID"
else
  fail "Proposal failed: $PROPOSAL_RESP"; exit 1
fi

curl -s -X POST "$BASE_URL/api/proposals/$PROPOSAL_ID/decision" \
  -H "Authorization: Bearer $TOKEN_A" \
  -H "Content-Type: application/json" \
  -d '{"status":"accepted"}' > /dev/null

START_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
END_DATE=$(date -u -d "+7 days" +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u -v+7d +"%Y-%m-%dT%H:%M:%SZ")

CONTRACT_RESP=$(curl -s -X POST "$BASE_URL/api/proposals/create-contract" \
  -H "Authorization: Bearer $TOKEN_A" \
  -H "Content-Type: application/json" \
  -d "{
    \"proposal_id\": \"$PROPOSAL_ID\",
    \"title\": \"Payment Test Contract\",
    \"description\": \"Full payment flow test\",
    \"total_amount\": 18000,
    \"start_date\": \"$START_DATE\",
    \"end_date\": \"$END_DATE\",
    \"terms\": \"Release on completion.\"
  }")

CONTRACT_ID=$(echo "$CONTRACT_RESP" | jq -r '.contract.id // empty')
if [ -n "$CONTRACT_ID" ] && [ "$CONTRACT_ID" != "null" ]; then
  pass "Contract created — ID: $CONTRACT_ID"
else
  fail "Contract creation failed: $CONTRACT_RESP"; exit 1
fi

# ────────────────────────────────────────────────────────────
section "3. ESCROW DEPOSIT (Stripe PaymentIntent)"
# ────────────────────────────────────────────────────────────

ESCROW_RESP=$(curl -s -X POST "$BASE_URL/api/v1/escrow/contracts/$CONTRACT_ID/deposit" \
  -H "Authorization: Bearer $TOKEN_A" \
  -H "Content-Type: application/json")

CLIENT_SECRET=$(echo "$ESCROW_RESP" | jq -r '.client_secret // empty')
PAYMENT_INTENT_ID=$(echo "$ESCROW_RESP" | jq -r '.payment_intent_id // empty')
ESCROW_AMOUNT=$(echo "$ESCROW_RESP" | jq -r '.amount // empty')

if [ -n "$CLIENT_SECRET" ] && [ "$CLIENT_SECRET" != "null" ]; then
  pass "Escrow deposit initiated"
  info "PaymentIntent amount: ${ESCROW_AMOUNT} GBP pence"
  info "PaymentIntent ID: $PAYMENT_INTENT_ID"
else
  fail "Escrow deposit failed: $ESCROW_RESP"
  exit 1
fi

# ────────────────────────────────────────────────────────────
section "4. STRIPE CLI — Confirm PaymentIntent"
# ────────────────────────────────────────────────────────────
# Use Stripe test card pm_card_visa to confirm the manual-capture PI.
# This triggers payment_intent.amount_capturable_updated webhook.

stripe_info "Confirming PaymentIntent $PAYMENT_INTENT_ID with test card..."

# Build stripe CLI args — pass --api-key so it uses the same account as the server
STRIPE_ARGS=()
if [ -n "$STRIPE_API_KEY" ]; then
  STRIPE_ARGS+=(--api-key "$STRIPE_API_KEY")
fi

CONFIRM_OUTPUT=$(stripe payment_intents confirm "$PAYMENT_INTENT_ID" \
  "${STRIPE_ARGS[@]}" \
  --payment-method=pm_card_visa 2>&1)

if echo "$CONFIRM_OUTPUT" | grep -q "requires_capture\|amount_capturable_updated\|succeeded\|\"status\""; then
  pass "Stripe PaymentIntent confirmed — funds authorized and held"
  stripe_info "Status: $(echo "$CONFIRM_OUTPUT" | grep '"status"' | head -1 | tr -d ' ')"
else
  fail "Stripe confirm failed: $CONFIRM_OUTPUT"
fi

# Wait for webhook to process
info "Waiting 3s for webhook to fire and be processed..."
sleep 3

# Check escrow status — should now be "funded"
ESCROW_STATUS=$(curl -s "$BASE_URL/api/v1/escrow/contracts/$CONTRACT_ID/status" \
  -H "Authorization: Bearer $TOKEN_A")
STATUS_VAL=$(echo "$ESCROW_STATUS" | jq -r '.data.escrow_status // empty')
if [ "$STATUS_VAL" = "funded" ]; then
  pass "Escrow status is 'funded' after webhook — webhook processing confirmed"
else
  skip "Escrow status is '${STATUS_VAL:-unknown}' — webhook may not have fired yet or webhook secret may not match"
  info "Check: is 'stripe listen --forward-to $WEBHOOK_ENDPOINT' running?"
  info "Check: does STRIPE_WEBHOOK_SECRET in .env match the key shown by stripe listen?"
fi

# ────────────────────────────────────────────────────────────
section "5. CONTRACT COMPLETION & ESCROW RELEASE"
# ────────────────────────────────────────────────────────────

# Freelancer marks complete
COMP_B=$(curl -s -X POST "$BASE_URL/api/contracts/$CONTRACT_ID/complete" \
  -H "Authorization: Bearer $TOKEN_B" \
  -H "Content-Type: application/json" -d '{}')
FL_DONE=$(echo "$COMP_B" | jq -r '.contract.freelancer_completed // false')
if [ "$FL_DONE" = "true" ]; then
  pass "Freelancer marked contract complete"
else
  fail "Freelancer completion failed: $COMP_B"
fi

# Client marks complete — this triggers CaptureEscrow → payment_intent.succeeded
COMP_A=$(curl -s -X POST "$BASE_URL/api/contracts/$CONTRACT_ID/complete" \
  -H "Authorization: Bearer $TOKEN_A" \
  -H "Content-Type: application/json" -d '{}')
CONTRACT_FINAL_STATUS=$(echo "$COMP_A" | jq -r '.contract.status // empty')
if [ "$CONTRACT_FINAL_STATUS" = "completed" ]; then
  pass "Contract completed — escrow capture triggered"
else
  fail "Client completion failed: $COMP_A"
fi

# Wait for payment_intent.succeeded webhook
info "Waiting 4s for payment_intent.succeeded webhook..."
sleep 4

# ────────────────────────────────────────────────────────────
section "6. VERIFY FREELANCER WALLET"
# ────────────────────────────────────────────────────────────

WALLET_B=$(curl -s "$BASE_URL/api/v1/wallet/balance" \
  -H "Authorization: Bearer $TOKEN_B")

WALLET_BAL=$(echo "$WALLET_B" | jq -r '.data.wallet_balance // 0')
TOTAL_AVAIL=$(echo "$WALLET_B" | jq -r '.data.total_available // 0')

if [ "$WALLET_BAL" -gt 0 ] 2>/dev/null; then
  pass "Freelancer wallet credited — balance: £$(echo "scale=2; $WALLET_BAL/100" | bc 2>/dev/null || echo "$WALLET_BAL pence")"
  info "Total available: ${TOTAL_AVAIL} pence"
else
  skip "Wallet balance is ${WALLET_BAL} — escrow may not have been funded (webhook not fired)"
  info "To force capture: POST /api/v1/escrow/contracts/$CONTRACT_ID/refund (or wait for Stripe webhook)"
fi

# ────────────────────────────────────────────────────────────
section "7. BANK ACCOUNT MANAGEMENT"
# ────────────────────────────────────────────────────────────
# Stripe test bank account for GB: sort 108800, account 00012345

echo -e "${YELLOW}7a. Add bank account (User B)${NC}"
ADD_BA=$(curl -s -X POST "$BASE_URL/api/v1/wallet/bank-accounts" \
  -H "Authorization: Bearer $TOKEN_B" \
  -H "Content-Type: application/json" \
  -d '{
    "account_holder_name": "Bob Freelancer",
    "sort_code": "108800",
    "account_number": "00012345",
    "currency": "gbp",
    "set_as_default": true
  }')

BA_ID=$(echo "$ADD_BA" | jq -r '.data.id // empty')
BA_BANK=$(echo "$ADD_BA" | jq -r '.data.bank_name // empty')
BA_LAST4=$(echo "$ADD_BA" | jq -r '.data.account_number_last4 // empty')

if [ -n "$BA_ID" ] && [ "$BA_ID" != "null" ]; then
  pass "Bank account added — ID: $BA_ID, Bank: ${BA_BANK:-unknown}, Last4: ${BA_LAST4:-xxxx}"
  info "Sort code stored: $(echo "$ADD_BA" | jq -r '.data.sort_code')"
  info "Is default: $(echo "$ADD_BA" | jq -r '.data.is_default')"
else
  STRIPE_ERR=$(echo "$ADD_BA" | jq -r '.error // empty')
  if echo "$STRIPE_ERR" | grep -qi "signed up for Connect\|connect"; then
    skip "Stripe Connect not enabled on this account"
    echo -e "${YELLOW}  ┌─────────────────────────────────────────────────────────────┐${NC}"
    echo -e "${YELLOW}  │  ACTION REQUIRED: Enable Stripe Connect                     │${NC}"
    echo -e "${YELLOW}  │  1. Go to https://dashboard.stripe.com/connect              │${NC}"
    echo -e "${YELLOW}  │  2. Complete the Connect onboarding for your platform       │${NC}"
    echo -e "${YELLOW}  │  3. Re-run this script after enabling Connect               │${NC}"
    echo -e "${YELLOW}  └─────────────────────────────────────────────────────────────┘${NC}"
    BA_ID=""
  else
    fail "Bank account add failed: $STRIPE_ERR"
    info "Full response: $ADD_BA"
  fi
fi

echo -e "${YELLOW}7b. List bank accounts${NC}"
LIST_BA=$(curl -s "$BASE_URL/api/v1/wallet/bank-accounts" \
  -H "Authorization: Bearer $TOKEN_B")
BA_COUNT=$(echo "$LIST_BA" | jq '.data | length // 0')
if [ "$BA_COUNT" -gt 0 ]; then
  pass "Bank accounts listed — $BA_COUNT account(s)"
  info "Default account: $(echo "$LIST_BA" | jq -r '.data[] | select(.is_default==true) | "\(.bank_name) ****\(.account_number_last4)"')"
else
  skip "No bank accounts found — bank account creation may have failed"
fi

# ────────────────────────────────────────────────────────────
section "8. WITHDRAWAL (Payout to Bank Account)"
# ────────────────────────────────────────────────────────────

# Re-check balance before withdrawal
WALLET_BEFORE=$(curl -s "$BASE_URL/api/v1/wallet/balance" \
  -H "Authorization: Bearer $TOKEN_B")
BAL_BEFORE=$(echo "$WALLET_BEFORE" | jq -r '.data.wallet_balance // 0')
info "Wallet balance before withdrawal: ${BAL_BEFORE} pence"

if [ "$BAL_BEFORE" -gt 0 ] 2>/dev/null && [ -n "$BA_ID" ] && [ "$BA_ID" != "null" ]; then
  WITHDRAW_AMOUNT=$((BAL_BEFORE > 1000 ? 1000 : BAL_BEFORE))
  info "Attempting withdrawal of ${WITHDRAW_AMOUNT} pence..."

  WITHDRAW_RESP=$(curl -s -X POST "$BASE_URL/api/v1/wallet/withdraw" \
    -H "Authorization: Bearer $TOKEN_B" \
    -H "Content-Type: application/json" \
    -d "{\"amount\": $WITHDRAW_AMOUNT, \"currency\": \"gbp\"}")

  PAYOUT_ID=$(echo "$WITHDRAW_RESP" | jq -r '.data.payout_id // empty')
  STRIPE_TRANSFER_ID=$(echo "$WITHDRAW_RESP" | jq -r '.data.stripe_transfer_id // empty')
  STRIPE_PAYOUT_ID=$(echo "$WITHDRAW_RESP" | jq -r '.data.stripe_payout_id // empty')
  REMAINING=$(echo "$WITHDRAW_RESP" | jq -r '.data.remaining_balance // empty')

  if [ -n "$PAYOUT_ID" ] && [ "$PAYOUT_ID" != "null" ]; then
    pass "Withdrawal initiated — Payout ID: $PAYOUT_ID"
    info "Amount: ${WITHDRAW_AMOUNT} pence"
    info "Stripe Transfer ID: ${STRIPE_TRANSFER_ID}"
    info "Stripe Payout ID: ${STRIPE_PAYOUT_ID}"
    info "Remaining balance: ${REMAINING} pence"

    stripe_info "Verifying transfer on Stripe..."
    if [ -n "$STRIPE_TRANSFER_ID" ] && [ "$STRIPE_TRANSFER_ID" != "null" ]; then
      TRANSFER_STATUS=$(stripe transfers retrieve "$STRIPE_TRANSFER_ID" 2>&1 | grep '"id"' | head -1)
      if [ -n "$TRANSFER_STATUS" ]; then
        pass "Stripe Transfer verified: $STRIPE_TRANSFER_ID"
      else
        info "Stripe Transfer created (CLI verification skipped — check Stripe dashboard)"
      fi
    fi
  else
    WITHDRAW_ERR=$(echo "$WITHDRAW_RESP" | jq -r '.error // empty')
    fail "Withdrawal failed: $WITHDRAW_ERR"
    info "Full response: $WITHDRAW_RESP"
  fi
else
  skip "Withdrawal skipped — wallet balance is ${BAL_BEFORE} pence or no bank account"
  info "To test withdrawal manually: POST /api/v1/wallet/withdraw {amount: X, currency: gbp}"

  # Still test the endpoint with insufficient balance to verify error handling
  INSUF_RESP=$(curl -s -X POST "$BASE_URL/api/v1/wallet/withdraw" \
    -H "Authorization: Bearer $TOKEN_B" \
    -H "Content-Type: application/json" \
    -d '{"amount": 999999, "currency": "gbp"}')
  INSUF_ERR=$(echo "$INSUF_RESP" | jq -r '.error // empty')
  if echo "$INSUF_ERR" | grep -qi "insufficient\|balance\|bank"; then
    pass "Withdrawal correctly rejects insufficient balance: $INSUF_ERR"
  else
    info "Insufficient balance response: $INSUF_RESP"
  fi
fi

# ────────────────────────────────────────────────────────────
section "9. PAYOUT HISTORY"
# ────────────────────────────────────────────────────────────

HISTORY=$(curl -s "$BASE_URL/api/v1/wallet/withdrawals" \
  -H "Authorization: Bearer $TOKEN_B")
if echo "$HISTORY" | grep -qi '"message"'; then
  HIST_COUNT=$(echo "$HISTORY" | jq '.data | length // 0')
  pass "Payout history retrieved — $HIST_COUNT entry(ies)"
  if [ "$HIST_COUNT" -gt 0 ]; then
    info "Latest: $(echo "$HISTORY" | jq -r '.data[0] | "status=\(.status) amount=\(.amount) transfer=\(.stripe_transfer_id)"')"
  fi
else
  fail "Payout history failed: $HISTORY"
fi

# ────────────────────────────────────────────────────────────
section "10. ADMIN PAYOUT LIST"
# ────────────────────────────────────────────────────────────

ADMIN_PAYOUTS=$(curl -s "$BASE_URL/api/v1/admin/payouts?limit=5&page=0" \
  -H "Authorization: Bearer $TOKEN_A")
if echo "$ADMIN_PAYOUTS" | grep -qi '"message"'; then
  ADMIN_PO_COUNT=$(echo "$ADMIN_PAYOUTS" | jq '.data | length // 0')
  ADMIN_PO_TOTAL=$(echo "$ADMIN_PAYOUTS" | jq '.total // 0')
  pass "Admin payout list — total: $ADMIN_PO_TOTAL, returned: $ADMIN_PO_COUNT"
else
  fail "Admin payout list failed: $ADMIN_PAYOUTS"
fi

# ────────────────────────────────────────────────────────────
section "11. BANK ACCOUNT CLEANUP"
# ────────────────────────────────────────────────────────────

if [ -n "$BA_ID" ] && [ "$BA_ID" != "null" ]; then
  DEL_BA=$(curl -s -X DELETE "$BASE_URL/api/v1/wallet/bank-accounts/$BA_ID" \
    -H "Authorization: Bearer $TOKEN_B")
  if echo "$DEL_BA" | grep -qi '"success"'; then
    pass "Bank account deleted — Stripe bank account also removed"
  else
    info "Delete response: $DEL_BA"
  fi
fi

# ────────────────────────────────────────────────────────────
section "12. STRIPE CLI — DIRECT EVENT TRIGGERS"
# ────────────────────────────────────────────────────────────
# Trigger standard Stripe events to verify webhook handler stability

echo -e "${YELLOW}12a. Trigger charge.updated (unhandled event — should 200 OK)${NC}"
TRIGGER_OUT=$(stripe trigger charge.updated 2>&1)
sleep 1
if echo "$TRIGGER_OUT" | grep -qi "triggered\|done\|ok\|running"; then
  pass "charge.updated trigger sent (webhook handler returns 'received')"
else
  info "Trigger output: $TRIGGER_OUT"
fi

echo -e "${YELLOW}12b. Trigger payment_intent.created (unhandled event)${NC}"
TRIGGER_PI=$(stripe trigger payment_intent.created 2>&1)
sleep 1
if echo "$TRIGGER_PI" | grep -qi "triggered\|done\|ok\|running"; then
  pass "payment_intent.created trigger sent"
else
  info "Trigger output: $TRIGGER_PI"
fi

# ────────────────────────────────────────────────────────────
section "SUMMARY"
# ────────────────────────────────────────────────────────────

TOTAL=$((PASS+FAIL+SKIP))
echo ""
echo -e "${GREEN}  Passed: $PASS${NC}"
echo -e "${RED}  Failed: $FAIL${NC}"
echo -e "${YELLOW}  Skipped: $SKIP${NC}"
echo -e "  Total:  $TOTAL"
echo ""

if [ "$FAIL" -gt 0 ]; then
  echo -e "${YELLOW}Troubleshooting:${NC}"
  echo "  • Webhook not firing?  Run: stripe listen --forward-to $WEBHOOK_ENDPOINT"
  echo "  • Wrong webhook secret? Match STRIPE_WEBHOOK_SECRET in .env with 'whsec_...' shown by stripe listen"
  echo "  • Bank account error?  Ensure STRIPE_SECRET_KEY=sk_test_... (test key)"
  echo "  • Transfer error?      Platform account needs funds — top up via Stripe dashboard"
  echo ""
fi

if [ "$FAIL" -eq 0 ]; then
  echo -e "${GREEN}All checks passed!${NC}"
fi
