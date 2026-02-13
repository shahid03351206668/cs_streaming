#!/bin/bash

# Portfolio and Certification API Test Script

BASE_URL="http://192.168.18.52:8080"

echo "======================================"
echo "Portfolio & Certification API Tests"
echo "======================================"
echo ""

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Test 1: Register a test user
echo -e "${YELLOW}Test 1: Register test user${NC}"
TIMESTAMP=$(date +%s)
REGISTER_RESPONSE=$(curl -s -X POST "$BASE_URL/api/auth/register" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "first_name=Portfolio" \
    -d "last_name=Tester" \
    -d "email=portfolio_${TIMESTAMP}@test.com" \
    -d "password=password123" \
    -d "phone_number=+1${TIMESTAMP}")

USER_TOKEN=$(echo "$REGISTER_RESPONSE" | jq -r '.tokens.access_token // empty')
USER_ID=$(echo "$REGISTER_RESPONSE" | jq -r '.user.id // empty')

if [ -n "$USER_TOKEN" ] && [ "$USER_TOKEN" != "null" ]; then
    echo -e "${GREEN}✓ Test 1 Passed${NC}"
    echo "User ID: $USER_ID"
else
    echo -e "${RED}✗ Test 1 Failed${NC}"
    echo "Response: $REGISTER_RESPONSE"
    exit 1
fi
echo ""

# Test 2: Add a portfolio with media files
echo -e "${YELLOW}Test 2: Add portfolio with media files${NC}"

# Create a test image file
cat > /tmp/test_image.jpg << 'EOF'
/9j/4AAQSkZJRgABAQEAYABgAAD/2wBDAAEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQH/2wBDAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQH/wAARCABAAEADASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWm5ybnJ2eoqOkpaanqKmqsrO0tba2uLm6wsPExcbHyMnK0tPU1dbW2Nna4uPk5ebn6Onq8vP09fb2+Pn6/8QAHwEAAwEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlbaWmJmaoqOkpaanqKmqsrO0tba2uLm6wsPExcbHyMnK0tPU1dbW2Nna4uPk5ebn6Onq8vP09fb2+Pn6/9oADAMBAAIRAxEAPwD+/KKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigD/9k=
EOF

ADD_PORTFOLIO_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/user/$USER_ID/portfolio" \
    -H "Authorization: Bearer $USER_TOKEN" \
    -F "title=My Portfolio Project" \
    -F "description=This is a test portfolio project" \
    -F "project_url=https://example.com/project" \
    -F "media=@/tmp/test_image.jpg")

echo "Response: $ADD_PORTFOLIO_RESPONSE"

PORTFOLIO_ID=$(echo "$ADD_PORTFOLIO_RESPONSE" | jq -r '.data.id // empty')
MEDIA_COUNT=$(echo "$ADD_PORTFOLIO_RESPONSE" | jq '.data.media | length // 0')

if [ -n "$PORTFOLIO_ID" ] && [ "$PORTFOLIO_ID" != "null" ]; then
    echo -e "${GREEN}✓ Test 2 Passed - Portfolio created with ID: $PORTFOLIO_ID${NC}"
    echo "Media files uploaded: $MEDIA_COUNT"
else
    echo -e "${RED}✗ Test 2 Failed${NC}"
fi
echo ""

# Test 3: Get portfolio with media
echo -e "${YELLOW}Test 3: Get portfolio with media${NC}"
GET_PORTFOLIO_RESPONSE=$(curl -s -X GET "$BASE_URL/api/v1/user/$USER_ID/portfolio" \
    -H "Authorization: Bearer $USER_TOKEN")

echo "Response: $GET_PORTFOLIO_RESPONSE"

PORTFOLIO_COUNT=$(echo "$GET_PORTFOLIO_RESPONSE" | jq '.data | length // 0')
FIRST_PORTFOLIO_MEDIA=$(echo "$GET_PORTFOLIO_RESPONSE" | jq '.data[0].media | length // 0')

if [ "$PORTFOLIO_COUNT" -gt 0 ] && [ "$FIRST_PORTFOLIO_MEDIA" -gt 0 ]; then
    echo -e "${GREEN}✓ Test 3 Passed - Portfolio retrieved with $FIRST_PORTFOLIO_MEDIA media files${NC}"
else
    echo -e "${RED}✗ Test 3 Failed - Portfolio count: $PORTFOLIO_COUNT, Media count: $FIRST_PORTFOLIO_MEDIA${NC}"
fi
echo ""

# Test 4: Add a certification
echo -e "${YELLOW}Test 4: Add certification with image${NC}"

ADD_CERT_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/user/$USER_ID/certifications" \
    -H "Authorization: Bearer $USER_TOKEN" \
    -F "name=AWS Solutions Architect" \
    -F "issuing_organization=Amazon Web Services" \
    -F "issue_date=2024-01-15" \
    -F "expiration_date=2026-01-15" \
    -F "image=@/tmp/test_image.jpg")

echo "Response: $ADD_CERT_RESPONSE"

CERT_ID=$(echo "$ADD_CERT_RESPONSE" | jq -r '.id // .data.id // empty')
CERT_IMAGE_URL=$(echo "$ADD_CERT_RESPONSE" | jq -r '.image_url // empty')

if [ -n "$CERT_ID" ] && [ "$CERT_ID" != "null" ]; then
    echo -e "${GREEN}✓ Test 4 Passed - Certification created with ID: $CERT_ID${NC}"
    echo "Image URL: $CERT_IMAGE_URL"
else
    echo -e "${RED}✗ Test 4 Failed${NC}"
fi
echo ""

# Test 5: Get certifications
echo -e "${YELLOW}Test 5: Get certifications${NC}"
GET_CERT_RESPONSE=$(curl -s -X GET "$BASE_URL/api/v1/user/$USER_ID/certifications" \
    -H "Authorization: Bearer $USER_TOKEN")

echo "Response: $GET_CERT_RESPONSE"

CERT_COUNT=$(echo "$GET_CERT_RESPONSE" | jq '.data | length // 0')
FIRST_CERT_NAME=$(echo "$GET_CERT_RESPONSE" | jq -r '.data[0].name // empty')
FIRST_CERT_IMAGE=$(echo "$GET_CERT_RESPONSE" | jq -r '.data[0].image_url // empty')

if [ "$CERT_COUNT" -gt 0 ]; then
    echo -e "${GREEN}✓ Test 5 Passed - Retrieved $CERT_COUNT certification(s)${NC}"
    echo "First certification: $FIRST_CERT_NAME"
    echo "Image URL: $FIRST_CERT_IMAGE"
else
    echo -e "${RED}✗ Test 5 Failed - No certifications found${NC}"
fi
echo ""

# Test 6: Update portfolio
echo -e "${YELLOW}Test 6: Update portfolio with additional media${NC}"

UPDATE_PORTFOLIO_RESPONSE=$(curl -s -X PUT "$BASE_URL/api/v1/user/$USER_ID/portfolio/$PORTFOLIO_ID" \
    -H "Authorization: Bearer $USER_TOKEN" \
    -F "title=Updated Portfolio Title" \
    -F "description=Updated description" \
    -F "project_url=https://example.com/updated" \
    -F "new_media=@/tmp/test_image.jpg")

echo "Response: $UPDATE_PORTFOLIO_RESPONSE"

UPDATED_MEDIA_COUNT=$(echo "$UPDATE_PORTFOLIO_RESPONSE" | jq '.data.media | length // 0')

if [ "$UPDATED_MEDIA_COUNT" -gt 0 ]; then
    echo -e "${GREEN}✓ Test 6 Passed - Portfolio updated with $UPDATED_MEDIA_COUNT media files${NC}"
else
    echo -e "${YELLOW}⚠ Test 6 - Media count: $UPDATED_MEDIA_COUNT${NC}"
fi
echo ""

# Test 7: Get portfolio by public user ID (without auth)
echo -e "${YELLOW}Test 7: Get portfolio by public user ID (no auth)${NC}"
PUBLIC_PORTFOLIO_RESPONSE=$(curl -s -X GET "$BASE_URL/api/v1/user/$USER_ID/portfolio")

echo "Response: $PUBLIC_PORTFOLIO_RESPONSE"

PUBLIC_PORTFOLIO_COUNT=$(echo "$PUBLIC_PORTFOLIO_RESPONSE" | jq '.data | length // 0')
PUBLIC_MEDIA_COUNT=$(echo "$PUBLIC_PORTFOLIO_RESPONSE" | jq '.data[0].media | length // 0')

if [ "$PUBLIC_PORTFOLIO_COUNT" -gt 0 ] && [ "$PUBLIC_MEDIA_COUNT" -gt 0 ]; then
    echo -e "${GREEN}✓ Test 7 Passed - Public portfolio access with $PUBLIC_MEDIA_COUNT media files${NC}"
else
    echo -e "${RED}✗ Test 7 Failed - Portfolio count: $PUBLIC_PORTFOLIO_COUNT, Media count: $PUBLIC_MEDIA_COUNT${NC}"
fi
echo ""

# Cleanup
rm -f /tmp/test_image.jpg

echo "======================================"
echo "Tests Completed"
echo "======================================"
