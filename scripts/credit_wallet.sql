-- ═══════════════════════════════════════════════════════════════════════════
-- QUICK WALLET CREDIT SCRIPT FOR TESTING
-- ═══════════════════════════════════════════════════════════════════════════
-- 
-- PURPOSE: Manually credit a user's wallet for testing withdrawal functionality
-- 
-- USAGE:
--   1. Find your user ID: SELECT id, email FROM users WHERE email = 'your-email@example.com';
--   2. Replace 'YOUR-USER-ID-HERE' with actual UUID
--   3. Run this script in your PostgreSQL client
--
-- IMPORTANT: This is for TEST ENVIRONMENT ONLY!
-- ═══════════════════════════════════════════════════════════════════════════

-- Step 1: Find your user ID (uncomment and run first)
-- SELECT id, email, first_name, last_name 
-- FROM users 
-- WHERE email = 'test@example.com';

-- Step 2: Credit wallet with ₦100,000.00 (10,000,000 kobo)
DO $$
DECLARE
    v_user_id UUID := 'YOUR-USER-ID-HERE'; -- ← Replace with your actual user ID
    v_wallet_id UUID;
    v_amount_kobo BIGINT := 10000000; -- ₦100,000.00
    v_current_balance BIGINT;
    v_new_balance BIGINT;
BEGIN
    -- Get wallet ID and current balance
    SELECT id, main_balance 
    INTO v_wallet_id, v_current_balance
    FROM wallets 
    WHERE user_id = v_user_id;

    IF v_wallet_id IS NULL THEN
        RAISE EXCEPTION 'No wallet found for user %', v_user_id;
    END IF;

    -- Calculate new balance
    v_new_balance := v_current_balance + v_amount_kobo;

    -- Update wallet balance
    UPDATE wallets 
    SET 
        main_balance = v_new_balance,
        updated_at = NOW()
    WHERE id = v_wallet_id;

    -- Create transaction record
    INSERT INTO wallet_transactions (
        id,
        wallet_id,
        type,
        amount,
        balance_before,
        balance_after,
        reference,
        description,
        status,
        provider_reference,
        created_at,
        updated_at
    ) VALUES (
        gen_random_uuid(),
        v_wallet_id,
        'deposit',
        v_amount_kobo,
        v_current_balance,
        v_new_balance,
        'TEST-CREDIT-' || TO_CHAR(NOW(), 'YYYYMMDD-HH24MISS'),
        'Manual test credit for withdrawal testing',
        'completed',
        'MANUAL-CREDIT',
        NOW(),
        NOW()
    );

    -- Show result
    RAISE NOTICE '✓ Wallet credited successfully!';
    RAISE NOTICE '  User ID: %', v_user_id;
    RAISE NOTICE '  Previous Balance: ₦%', (v_current_balance::DECIMAL / 100);
    RAISE NOTICE '  Amount Credited: ₦%', (v_amount_kobo::DECIMAL / 100);
    RAISE NOTICE '  New Balance: ₦%', (v_new_balance::DECIMAL / 100);
END $$;

-- Step 3: Verify the credit (run after script)
-- SELECT 
--     w.id as wallet_id,
--     u.email,
--     w.main_balance / 100.0 as balance_naira,
--     w.updated_at
-- FROM wallets w
-- JOIN users u ON u.id = w.user_id
-- WHERE u.id = 'YOUR-USER-ID-HERE';

-- Step 4: View transaction history
-- SELECT 
--     type,
--     amount / 100.0 as amount_naira,
--     balance_before / 100.0 as before_naira,
--     balance_after / 100.0 as after_naira,
--     reference,
--     status,
--     created_at
-- FROM wallet_transactions
-- WHERE wallet_id IN (SELECT id FROM wallets WHERE user_id = 'YOUR-USER-ID-HERE')
-- ORDER BY created_at DESC
-- LIMIT 10;
