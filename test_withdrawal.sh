#!/bin/bash

# Withdrawal Test Runner
# Automates common withdrawal test scenarios

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
API_BASE="http://localhost:8080/api/v1"
TOKEN=""
USER_EMAIL=""

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

log_error() {
    echo -e "${RED}[✗]${NC} $1"
    TESTS_FAILED=$((TESTS_FAILED + 1))
}

log_warning() {
    echo -e "${YELLOW}[!]${NC} $1"
}

# Setup test user and get token
setup_test_user() {
    log_info "Setting up test user..."
    
    USER_EMAIL="test-$(date +%s)@example.com"
    
    RESPONSE=$(curl -s "$API_BASE/auth/register" \
        -H "Content-Type: application/json" \
        -d '{
            "first_name": "Test",
            "last_name": "User",
            "email": "'"$USER_EMAIL"'",
            "phone": "+234801234'"$(date +%s | tail -c 5)"'",
            "password": "SecureP@ss123"
        }')
    
    TOKEN=$(echo $RESPONSE | jq -r '.data.access_token')
    
    if [ "$TOKEN" != "null" ] && [ -n "$TOKEN" ]; then
        log_success "Test user created: $USER_EMAIL"
        return 0
    else
        log_error "Failed to create test user"
        echo $RESPONSE | jq '.'
        exit 1
    fi
}

# Fund test wallet
fund_wallet() {
    local AMOUNT=$1
    log_info "Funding wallet with ₦$(echo "scale=2; $AMOUNT/100" | bc)..."
    
    # In real test, you'd complete the payment flow
    # For now, we'll use a direct database insert or admin endpoint
    log_warning "Manual step: Complete payment for deposit"
}

# Get current balance
get_balance() {
    RESPONSE=$(curl -s "$API_BASE/wallet" \
        -H "Authorization: Bearer $TOKEN")
    
    BALANCE=$(echo $RESPONSE | jq -r '.data.main_balance')
    LOCKED=$(echo $RESPONSE | jq -r '.data.locked_balance')
    AVAILABLE=$((BALANCE - LOCKED))
    
    echo "Balance: ₦$(echo "scale=2; $BALANCE/100" | bc), Locked: ₦$(echo "scale=2; $LOCKED/100" | bc), Available: ₦$(echo "scale=2; $AVAILABLE/100" | bc)"
}

# Run test: Amount below minimum
test_minimum_amount() {
    TESTS_RUN=$((TESTS_RUN + 1))
    log_info "Test: Amount below minimum (₦400, expect 422)"
    
    RESPONSE=$(curl -s -w "\n%{http_code}" "$API_BASE/wallet/withdraw" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
            "amount_kobo": 40000,
            "account_number": "0123456789",
            "account_name": "Test User",
            "bank_code": "058"
        }')
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | head -n-1)
    ERROR_CODE=$(echo $BODY | jq -r '.code')
    
    if [ "$HTTP_CODE" == "422" ] && [ "$ERROR_CODE" == "minimum_withdrawal" ]; then
        log_success "Correctly rejected amount below minimum"
    else
        log_error "Expected 422 with code 'minimum_withdrawal', got $HTTP_CODE with code '$ERROR_CODE'"
    fi
}

# Run test: Amount above maximum
test_maximum_amount() {
    TESTS_RUN=$((TESTS_RUN + 1))
    log_info "Test: Amount above maximum (₦150,000, expect 422)"
    
    RESPONSE=$(curl -s -w "\n%{http_code}" "$API_BASE/wallet/withdraw" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
            "amount_kobo": 15000000,
            "account_number": "0123456789",
            "account_name": "Test User",
            "bank_code": "058"
        }')
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | head -n-1)
    ERROR_CODE=$(echo $BODY | jq -r '.code')
    
    if [ "$HTTP_CODE" == "422" ] && [ "$ERROR_CODE" == "maximum_withdrawal" ]; then
        log_success "Correctly rejected amount above maximum"
    else
        log_error "Expected 422 with code 'maximum_withdrawal', got $HTTP_CODE with code '$ERROR_CODE'"
    fi
}

# Run test: Insufficient balance
test_insufficient_balance() {
    TESTS_RUN=$((TESTS_RUN + 1))
    log_info "Test: Insufficient balance (₦999,999, expect 422)"
    
    RESPONSE=$(curl -s -w "\n%{http_code}" "$API_BASE/wallet/withdraw" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
            "amount_kobo": 99999900,
            "account_number": "0123456789",
            "account_name": "Test User",
            "bank_code": "058"
        }')
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | head -n-1)
    ERROR_CODE=$(echo $BODY | jq -r '.code')
    
    if [ "$HTTP_CODE" == "422" ] && [ "$ERROR_CODE" == "insufficient_funds" ]; then
        log_success "Correctly rejected insufficient balance"
    else
        log_error "Expected 422 with code 'insufficient_funds', got $HTTP_CODE with code '$ERROR_CODE'"
    fi
}

# Run test: Valid withdrawal
test_valid_withdrawal() {
    TESTS_RUN=$((TESTS_RUN + 1))
    log_info "Test: Valid withdrawal (₦500, expect 200)"
    
    RESPONSE=$(curl -s -w "\n%{http_code}" "$API_BASE/wallet/withdraw" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
            "amount_kobo": 50000,
            "account_number": "0123456789",
            "account_name": "Test User",
            "bank_code": "058"
        }')
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | head -n-1)
    SUCCESS=$(echo $BODY | jq -r '.success')
    
    if [ "$HTTP_CODE" == "200" ] && [ "$SUCCESS" == "true" ]; then
        log_success "Withdrawal initiated successfully"
        
        # Extract transaction details
        TX_ID=$(echo $BODY | jq -r '.data.id')
        TX_REF=$(echo $BODY | jq -r '.data.reference')
        log_info "Transaction ID: $TX_ID"
        log_info "Reference: $TX_REF"
    else
        log_error "Expected 200 with success=true, got $HTTP_CODE"
        echo $BODY | jq '.'
    fi
}

# Run test: Duplicate pending withdrawal
test_duplicate_pending() {
    TESTS_RUN=$((TESTS_RUN + 1))
    log_info "Test: Duplicate pending withdrawal (expect 422)"
    
    # Assume previous test created a pending withdrawal
    RESPONSE=$(curl -s -w "\n%{http_code}" "$API_BASE/wallet/withdraw" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
            "amount_kobo": 50000,
            "account_number": "0123456789",
            "account_name": "Test User",
            "bank_code": "058"
        }')
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | head -n-1)
    ERROR_CODE=$(echo $BODY | jq -r '.code')
    
    if [ "$HTTP_CODE" == "422" ] && [ "$ERROR_CODE" == "withdrawal_pending" ]; then
        log_success "Correctly rejected duplicate pending withdrawal"
    else
        log_error "Expected 422 with code 'withdrawal_pending', got $HTTP_CODE with code '$ERROR_CODE'"
    fi
}

# Run test: Unauthorized access
test_unauthorized() {
    TESTS_RUN=$((TESTS_RUN + 1))
    log_info "Test: Unauthorized access (no token, expect 401)"
    
    RESPONSE=$(curl -s -w "\n%{http_code}" "$API_BASE/wallet/withdraw" \
        -H "Content-Type: application/json" \
        -d '{
            "amount_kobo": 50000,
            "account_number": "0123456789",
            "account_name": "Test User",
            "bank_code": "058"
        }')
    
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    
    if [ "$HTTP_CODE" == "401" ]; then
        log_success "Correctly rejected unauthorized request"
    else
        log_error "Expected 401, got $HTTP_CODE"
    fi
}

# Main test execution
main() {
    echo "================================================"
    echo "   WITHDRAWAL API TEST SUITE"
    echo "================================================"
    echo ""
    
    # Setup
    setup_test_user
    echo ""
    
    # Current balance
    log_info "Current balance:"
    get_balance
    echo ""
    
    # Run tests
    log_info "Running validation tests..."
    test_unauthorized
    test_minimum_amount
    test_maximum_amount
    test_insufficient_balance
    echo ""
    
    log_info "Running functional tests..."
    log_warning "These tests require a funded wallet. Skipping for now."
    # Uncomment when wallet is funded:
    # test_valid_withdrawal
    # test_duplicate_pending
    echo ""
    
    # Summary
    echo "================================================"
    echo "   TEST SUMMARY"
    echo "================================================"
    echo "Tests run:    $TESTS_RUN"
    echo -e "Tests passed: ${GREEN}$TESTS_PASSED${NC}"
    echo -e "Tests failed: ${RED}$TESTS_FAILED${NC}"
    echo ""
    
    if [ $TESTS_FAILED -eq 0 ]; then
        echo -e "${GREEN}✓ All tests passed!${NC}"
        exit 0
    else
        echo -e "${RED}✗ Some tests failed${NC}"
        exit 1
    fi
}

# Run tests
main
