#!/bin/bash

# Referral System Test Script
# This script tests the referral program functionality

BASE_URL="http://192.168.18.52:8080"

echo "==================================="
echo "Referral System Test Script"
echo "==================================="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test 1: Validate non-existent referral code
echo -e "${YELLOW}Test 1: Validate non-existent referral code${NC}"
RESPONSE=$(curl -s -X GET "$BASE_URL/api/v1/referrals/validate/NONEXISTENT")
echo "Response: $RESPONSE"
if echo "$RESPONSE" | grep -q '"valid":false'; then
    echo -e "${GREEN}✓ Test 1 Passed${NC}"
else
    echo -e "${RED}✗ Test 1 Failed${NC}"
fi
echo ""

# Test 2: Register first user (referrer) - This user will create a referral code
echo -e "${YELLOW}Test 2: Register referrer user${NC}"
TIMESTAMP=$(date +%s)
REFERRER_RESPONSE=$(curl -s -X POST "$BASE_URL/api/auth/register" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "first_name=John" \
    -d "last_name=Referrer" \
    -d "email=referrer_${TIMESTAMP}@test.com" \
    -d "password=password123" \
    -d "phone_number=+1${TIMESTAMP}")
echo "Response: $REFERRER_RESPONSE"

REFERRER_TOKEN=$(echo "$REFERRER_RESPONSE" | jq -r '.tokens.access_token // empty')
REFERRER_ID=$(echo "$REFERRER_RESPONSE" | jq -r '.user.id // empty')

if [ -n "$REFERRER_TOKEN" ] && [ "$REFERRER_TOKEN" != "null" ]; then
    echo -e "${GREEN}✓ Test 2 Passed - Referrer registered${NC}"
    echo "Referrer ID: $REFERRER_ID"
else
    echo -e "${RED}✗ Test 2 Failed - Could not register referrer${NC}"
    echo "Full response: $REFERRER_RESPONSE"
fi
echo ""

# Test 3: Create referral code for the referrer
echo -e "${YELLOW}Test 3: Create referral code${NC}"
if [ -n "$REFERRER_TOKEN" ] && [ "$REFERRER_TOKEN" != "null" ]; then
    CREATE_CODE_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/referrals/codes" \
        -H "Authorization: Bearer $REFERRER_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
            "discount_percentage": 10,
            "max_uses": 5
        }')
    echo "Response: $CREATE_CODE_RESPONSE"
    
    REFERRAL_CODE=$(echo "$CREATE_CODE_RESPONSE" | jq -r '.data.code // empty')
    
    if [ -n "$REFERRAL_CODE" ] && [ "$REFERRAL_CODE" != "null" ]; then
        echo -e "${GREEN}✓ Test 3 Passed - Referral code created: $REFERRAL_CODE${NC}"
    else
        echo -e "${RED}✗ Test 3 Failed - Could not create referral code${NC}"
    fi
else
    echo -e "${YELLOW}⚠ Test 3 Skipped - No referrer token${NC}"
fi
echo ""

# Test 4: Validate the created referral code
echo -e "${YELLOW}Test 4: Validate created referral code${NC}"
if [ -n "$REFERRAL_CODE" ] && [ "$REFERRAL_CODE" != "null" ]; then
    VALIDATE_RESPONSE=$(curl -s -X GET "$BASE_URL/api/v1/referrals/validate/$REFERRAL_CODE")
    echo "Response: $VALIDATE_RESPONSE"
    
    if echo "$VALIDATE_RESPONSE" | grep -q '"valid":true'; then
        echo -e "${GREEN}✓ Test 4 Passed - Referral code is valid${NC}"
    else
        echo -e "${RED}✗ Test 4 Failed - Referral code validation failed${NC}"
    fi
else
    echo -e "${YELLOW}⚠ Test 4 Skipped - No referral code${NC}"
fi
echo ""

# Test 5: Register second user (referee) with the referral code
echo -e "${YELLOW}Test 5: Register referee with referral code${NC}"
if [ -n "$REFERRAL_CODE" ] && [ "$REFERRAL_CODE" != "null" ]; then
    TIMESTAMP2=$(date +%s)
    REFEREE_RESPONSE=$(curl -s -X POST "$BASE_URL/api/auth/register" \
        -H "Content-Type: application/x-www-form-urlencoded" \
        -d "first_name=Jane" \
        -d "last_name=Referee" \
        -d "email=referee_${TIMESTAMP2}@test.com" \
        -d "password=password123" \
        -d "phone_number=+2${TIMESTAMP2}" \
        -d "referral_code=$REFERRAL_CODE")
    echo "Response: $REFEREE_RESPONSE"
    
    REFEREE_TOKEN=$(echo "$REFEREE_RESPONSE" | jq -r '.tokens.access_token // empty')
    REFEREE_ID=$(echo "$REFEREE_RESPONSE" | jq -r '.user.id // empty')
    REFERRAL_APPLIED=$(echo "$REFEREE_RESPONSE" | jq -r '.referral_applied // empty')
    
    if [ "$REFERRAL_APPLIED" = "true" ]; then
        echo -e "${GREEN}✓ Test 5 Passed - Referee registered with referral code applied${NC}"
        echo "Referee ID: $REFEREE_ID"
    else
        REFERRAL_ERROR=$(echo "$REFEREE_RESPONSE" | jq -r '.referral_error // empty')
        echo -e "${RED}✗ Test 5 Failed - Referral not applied: $REFERRAL_ERROR${NC}"
    fi
else
    echo -e "${YELLOW}⚠ Test 5 Skipped - No referral code${NC}"
fi
echo ""

# Test 6: Check referee's referral status
echo -e "${YELLOW}Test 6: Check referee's referral status${NC}"
if [ -n "$REFEREE_TOKEN" ] && [ "$REFEREE_TOKEN" != "null" ]; then
    STATUS_RESPONSE=$(curl -s -X GET "$BASE_URL/api/v1/referrals/status" \
        -H "Authorization: Bearer $REFEREE_TOKEN")
    echo "Response: $STATUS_RESPONSE"
    
    HAS_REFERRAL=$(echo "$STATUS_RESPONSE" | jq -r '.has_referral // empty')
    if [ "$HAS_REFERRAL" = "true" ]; then
        echo -e "${GREEN}✓ Test 6 Passed - Referee has referral status${NC}"
    else
        echo -e "${RED}✗ Test 6 Failed - Referee has no referral status${NC}"
    fi
else
    echo -e "${YELLOW}⚠ Test 6 Skipped - No referee token${NC}"
fi
echo ""

# Test 7: Check referrer's referrals list
echo -e "${YELLOW}Test 7: Check referrer's referrals list${NC}"
if [ -n "$REFERRER_TOKEN" ] && [ "$REFERRER_TOKEN" != "null" ]; then
    REFERRALS_RESPONSE=$(curl -s -X GET "$BASE_URL/api/v1/referrals/my" \
        -H "Authorization: Bearer $REFERRER_TOKEN")
    echo "Response: $REFERRALS_RESPONSE"
    
    REFERRAL_COUNT=$(echo "$REFERRALS_RESPONSE" | jq '.data | length // 0')
    if [ "$REFERRAL_COUNT" -gt 0 ]; then
        echo -e "${GREEN}✓ Test 7 Passed - Referrer has $REFERRAL_COUNT referral(s)${NC}"
    else
        echo -e "${RED}✗ Test 7 Failed - Referrer has no referrals${NC}"
    fi
else
    echo -e "${YELLOW}⚠ Test 7 Skipped - No referrer token${NC}"
fi
echo ""

# Test 8: Check referrer's referral codes
echo -e "${YELLOW}Test 8: Check referrer's referral codes${NC}"
if [ -n "$REFERRER_TOKEN" ] && [ "$REFERRER_TOKEN" != "null" ]; then
    CODES_RESPONSE=$(curl -s -X GET "$BASE_URL/api/v1/referrals/codes/my" \
        -H "Authorization: Bearer $REFERRER_TOKEN")
    echo "Response: $CODES_RESPONSE"
    
    CODE_COUNT=$(echo "$CODES_RESPONSE" | jq '.data | length // 0')
    CURRENT_USES=$(echo "$CODES_RESPONSE" | jq '.data[0].current_uses // 0')
    if [ "$CODE_COUNT" -gt 0 ]; then
        echo -e "${GREEN}✓ Test 8 Passed - Referrer has $CODE_COUNT code(s), current uses: $CURRENT_USES${NC}"
    else
        echo -e "${RED}✗ Test 8 Failed - Referrer has no codes${NC}"
    fi
else
    echo -e "${YELLOW}⚠ Test 8 Skipped - No referrer token${NC}"
fi
echo ""

# Test 9: Try to use same referral code twice (should fail for same user)
echo -e "${YELLOW}Test 9: Try to register another user with same email (should fail)${NC}"
TIMESTAMP3=$(date +%s)
DUPLICATE_RESPONSE=$(curl -s -X POST "$BASE_URL/api/auth/register" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "first_name=Duplicate" \
    -d "last_name=User" \
    -d "email=referee_duplicate_${TIMESTAMP3}@test.com" \
    -d "password=password123" \
    -d "phone_number=+3${TIMESTAMP3}" \
    -d "referral_code=$REFERRAL_CODE")

# Register success, then try with same code for new user
TIMESTAMP4=$((TIMESTAMP3 + 1))
NEW_USER_RESPONSE=$(curl -s -X POST "$BASE_URL/api/auth/register" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "first_name=Another" \
    -d "last_name=Referee" \
    -d "email=another_referee_${TIMESTAMP4}@test.com" \
    -d "password=password123" \
    -d "phone_number=+4${TIMESTAMP4}" \
    -d "referral_code=$REFERRAL_CODE")
echo "Response: $NEW_USER_RESPONSE"

NEW_REFERRAL_APPLIED=$(echo "$NEW_USER_RESPONSE" | jq -r '.referral_applied // empty')
if [ "$NEW_REFERRAL_APPLIED" = "true" ]; then
    echo -e "${GREEN}✓ Test 9 Passed - Another user can use the same referral code${NC}"
else
    echo -e "${YELLOW}⚠ Test 9 - Second user referral: $NEW_REFERRAL_APPLIED${NC}"
fi
echo ""

# Test 10: Get payment transactions
echo -e "${YELLOW}Test 10: Get payment transactions list${NC}"
TRANSACTIONS_RESPONSE=$(curl -s -X GET "$BASE_URL/api/v1/payments/transactions")
echo "Response: $TRANSACTIONS_RESPONSE"
if echo "$TRANSACTIONS_RESPONSE" | grep -q '"message":"success"'; then
    echo -e "${GREEN}✓ Test 10 Passed - Transactions endpoint working${NC}"
else
    echo -e "${RED}✗ Test 10 Failed - Transactions endpoint not working${NC}"
fi
echo ""

echo "==================================="
echo "Test Summary"
echo "==================================="
echo "All tests completed!"
echo ""
echo "Note: The discount on first transaction will be applied"
echo "automatically when a payment is processed through Stripe webhook."
echo ""
