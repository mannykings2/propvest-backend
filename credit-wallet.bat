@echo off
REM Quick Wallet Credit Tool
REM Usage: credit-wallet.bat DEP-XXXXX

if "%1"=="" (
    echo.
    echo ┌─────────────────────────────────────────────────┐
    echo │   Quick Wallet Credit Tool                      │
    echo └─────────────────────────────────────────────────┘
    echo.
    echo Usage: credit-wallet.bat DEP-XXXXX
    echo.
    echo Example: credit-wallet.bat DEP-759B35C0-EB0
    echo.
    echo This manually triggers the webhook to credit the wallet
    echo after completing a payment on Paystack.
    echo.
    exit /b 1
)

echo.
echo ┌─────────────────────────────────────────────────┐
echo │   Triggering webhook for: %1
echo └─────────────────────────────────────────────────┘
echo.

go run tools\trigger_webhook.go %1

echo.
if %ERRORLEVEL% EQU 0 (
    echo ✅ Done! Check your wallet balance.
) else (
    echo ❌ Failed. Make sure the server is running.
)
echo.
