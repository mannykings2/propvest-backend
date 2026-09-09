# Setting Up ngrok for Webhook Testing

## What is ngrok?
ngrok creates a secure tunnel from the internet to your localhost, allowing Paystack to send webhooks to your local development server.

## Installation

### Option 1: Using Chocolatey (Recommended)
```powershell
choco install ngrok
```

### Option 2: Using Scoop
```powershell
scoop install ngrok
```

### Option 3: Manual Download
1. Download from https://ngrok.com/download
2. Extract `ngrok.exe` to a folder in your PATH
3. Or place it in your project root

## Setup Steps

### 1. Create ngrok Account (Free)
1. Go to https://dashboard.ngrok.com/signup
2. Sign up for a free account
3. Copy your authtoken from https://dashboard.ngrok.com/get-started/your-authtoken

### 2. Configure ngrok
```powershell
ngrok config add-authtoken <your_authtoken>
```

### 3. Start ngrok Tunnel
```powershell
# In a separate terminal, run:
ngrok http 8081
```

**Output:**
```
Session Status                online
Account                       your-email@example.com
Version                       3.x.x
Region                        United States (us)
Forwarding                    https://abc123def.ngrok-free.app -> http://localhost:8081
```

**Copy the `https://` URL** (e.g., `https://abc123def.ngrok-free.app`)

### 4. Update Your .env File
```bash
# Change BASE_URL to your ngrok URL
BASE_URL=https://abc123def.ngrok-free.app
```

### 5. Restart Your Server
```powershell
# Stop the server (Ctrl+C)
# Start it again
make run
```

### 6. Configure Paystack Webhook
1. Go to https://dashboard.paystack.com/#/settings/developer
2. Scroll to **Webhook Settings**
3. Set **Webhook URL** to: `https://abc123def.ngrok-free.app/api/v1/webhooks/payment`
4. Click **Save Changes**

### 7. Test the Full Flow!
Now when you:
1. Initiate deposit → Get payment URL
2. Pay on Paystack → Complete payment
3. **Webhook automatically sent** → Wallet credited immediately! ✅
4. Callback page shows success → Everything works!

## Important Notes

### ngrok URL Changes
- **Free ngrok URLs change every time you restart ngrok**
- You'll need to update `.env` and Paystack webhook URL each time
- **Solution:** Upgrade to ngrok paid plan for static URLs ($8/month)

### Keep ngrok Running
- ngrok must be running for webhooks to work
- Run it in a separate terminal window
- Don't close it while testing

### Production
- In production, you use your real domain (e.g., `https://api.propvest.com`)
- No ngrok needed

## Troubleshooting

### "Failed to complete tunnel connection"
- Check your internet connection
- Make sure you ran `ngrok config add-authtoken <token>`

### Webhook Not Received
1. Check ngrok is still running
2. Verify webhook URL in Paystack dashboard matches your ngrok URL
3. Check ngrok web interface: http://127.0.0.1:4040 (shows all requests)

### ngrok URL Expired
- Free URLs expire after 2 hours of inactivity
- Restart ngrok to get a new URL
- Update `.env` and Paystack webhook URL

## Alternative: ngrok in Background
Create a batch file to keep ngrok running:

**start-dev.bat:**
```batch
@echo off
echo Starting ngrok tunnel...
start "ngrok" ngrok http 8081
timeout /t 5
echo.
echo ngrok is running in the background
echo Check http://127.0.0.1:4040 for the ngrok URL
echo.
echo Starting API server...
make run
```

Then just run `start-dev.bat` to start everything!
