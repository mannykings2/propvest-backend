# Production Deployment Guide

Complete guide to deploying PropVest backend to production with HTTPS, security headers, and rate limiting.

---

## ✅ Security Features Implemented

All critical security features have been implemented:

### 1. ✅ Strong JWT Secrets
- **Status:** ✅ **COMPLETED**
- Cryptographically secure 64-byte random secrets generated
- Updated in `.env` file
- **Action:** Keep these secrets secure and never commit to git!

### 2. ✅ Rate Limiting
- **Status:** ✅ **IMPLEMENTED**
- In-memory rate limiter with token bucket algorithm
- Different limits for anonymous/authenticated/auth endpoints
- **File:** `internal/middleware/rate_limit.go`
- **Configured in:** `cmd/api/main.go`

**Current Limits:**
- Anonymous users: 100 requests/minute
- Authenticated users: 300 requests/minute
- Auth endpoints (login/register): 10 requests/minute

### 3. ✅ Security Headers
- **Status:** ✅ **IMPLEMENTED**
- X-Frame-Options, X-Content-Type-Options, X-XSS-Protection
- Strict-Transport-Security (HSTS) for production
- Content-Security-Policy
- **File:** `internal/middleware/security.go`
- **Configured in:** `cmd/api/main.go`

### 4. ✅ HTTPS/TLS Configuration
- **Status:** ✅ **READY (Configuration Files Created)**
- Nginx configuration: `nginx.conf`
- Caddy configuration: `Caddyfile` (simpler alternative)
- **Action Required:** Choose reverse proxy and deploy

---

## 🚀 Deployment Options

### Option 1: Nginx (Most Popular)
**Best for:** Maximum performance, battle-tested, most documentation

**Pros:**
- Most widely used
- Excellent performance
- Lots of community support
- Fine-grained control

**Cons:**
- Manual SSL certificate setup
- More complex configuration
- Manual certificate renewal setup

**Setup Time:** ~30 minutes

### Option 2: Caddy (Recommended for Simplicity)
**Best for:** Quick deployment, automatic SSL, less maintenance

**Pros:**
- Automatic HTTPS with Let's Encrypt
- Auto-renewal of certificates
- Simpler configuration
- Zero manual SSL setup

**Cons:**
- Less battle-tested than Nginx
- Slightly higher memory usage
- Smaller community

**Setup Time:** ~10 minutes

---

## 📋 Pre-Deployment Checklist

### Environment Configuration
- [ ] Update `JWT_SECRET` and `JWT_REFRESH_SECRET` (✅ Done)
- [ ] Set `APP_ENV=production`
- [ ] Update `BASE_URL` to production URL
- [ ] Update `ALLOWED_ORIGINS` to production frontend URLs
- [ ] Configure production database URL
- [ ] Set up production Redis instance
- [ ] Configure production email provider
- [ ] Configure production SMS provider
- [ ] Set up Paystack production keys
- [ ] Review all environment variables

### Security
- [ ] Strong JWT secrets set (✅ Done)
- [ ] Rate limiting enabled (✅ Done)
- [ ] Security headers configured (✅ Done)
- [ ] HTTPS/TLS configured
- [ ] Firewall rules configured
- [ ] Database access restricted
- [ ] Redis access restricted
- [ ] Review CORS allowed origins

### Infrastructure
- [ ] Server provisioned (VPS/Cloud)
- [ ] Domain name configured (DNS)
- [ ] SSL certificate obtained
- [ ] Reverse proxy installed (Nginx/Caddy)
- [ ] PostgreSQL installed/accessible
- [ ] Redis installed/accessible
- [ ] Monitoring set up
- [ ] Backup system configured

---

## 🔧 Quick Start: Caddy Deployment (Easiest)

### Step 1: Install Caddy

```bash
# Ubuntu/Debian
sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
sudo apt update
sudo apt install caddy
```

### Step 2: Configure DNS

Create an A record:
```
api.propvest.com → YOUR_SERVER_IP
```

### Step 3: Deploy Caddyfile

```bash
# Copy Caddyfile to server
scp Caddyfile user@your-server:/etc/caddy/Caddyfile

# Edit domain name
sudo nano /etc/caddy/Caddyfile
# Change api.propvest.com to your actual domain

# Reload Caddy (it will auto-obtain SSL!)
sudo systemctl reload caddy
```

### Step 4: Deploy Backend

```bash
# Build for Linux (on your dev machine)
GOOS=linux GOARCH=amd64 go build -o propvest-api cmd/api/main.go

# Copy to server
scp propvest-api user@your-server:/opt/propvest/
scp .env user@your-server:/opt/propvest/

# SSH to server
ssh user@your-server

# Create systemd service
sudo nano /etc/systemd/system/propvest-api.service
```

Add this content:
```ini
[Unit]
Description=PropVest API Server
After=network.target postgresql.service redis.service

[Service]
Type=simple
User=propvest
WorkingDirectory=/opt/propvest
ExecStart=/opt/propvest/propvest-api
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=propvest-api

# Environment
Environment="APP_ENV=production"

[Install]
WantedBy=multi-user.target
```

```bash
# Create user
sudo useradd -r -s /bin/false propvest
sudo chown -R propvest:propvest /opt/propvest

# Start service
sudo systemctl daemon-reload
sudo systemctl enable propvest-api
sudo systemctl start propvest-api

# Check status
sudo systemctl status propvest-api
```

### Step 5: Configure Firewall

```bash
sudo ufw allow 80/tcp    # HTTP (for Let's Encrypt)
sudo ufw allow 443/tcp   # HTTPS
sudo ufw allow 22/tcp    # SSH
sudo ufw enable
```

### Step 6: Verify Deployment

```bash
# Check SSL
curl https://api.propvest.com/health

# Check from browser
# Visit: https://api.propvest.com/health
```

**Done!** Caddy automatically handles SSL certificates and renewal.

---

## 🔧 Alternative: Nginx Deployment

### Step 1: Install Nginx

```bash
sudo apt update
sudo apt install nginx
```

### Step 2: Install Certbot (Let's Encrypt)

```bash
sudo apt install certbot python3-certbot-nginx
```

### Step 3: Configure DNS

Create an A record:
```
api.propvest.com → YOUR_SERVER_IP
```

### Step 4: Obtain SSL Certificate

```bash
# This will automatically configure Nginx
sudo certbot --nginx -d api.propvest.com

# Follow prompts:
# - Enter email address
# - Agree to terms
# - Choose to redirect HTTP to HTTPS (recommended)
```

### Step 5: Deploy Nginx Config

```bash
# Copy nginx.conf to server
scp nginx.conf user@your-server:/tmp/

# SSH to server
ssh user@your-server

# Move to sites-available
sudo mv /tmp/nginx.conf /etc/nginx/sites-available/propvest-api

# Create symlink
sudo ln -s /etc/nginx/sites-available/propvest-api /etc/nginx/sites-enabled/

# Test configuration
sudo nginx -t

# Reload Nginx
sudo systemctl reload nginx
```

### Step 6: Set Up Auto-Renewal

```bash
# Certbot installs a timer automatically
sudo systemctl status certbot.timer

# Test renewal (dry run)
sudo certbot renew --dry-run
```

### Step 7: Deploy Backend

Same as Caddy deployment (Step 4-5 above)

---

## 📊 Monitoring & Logging

### View Application Logs

```bash
# Backend logs
sudo journalctl -u propvest-api -f

# Nginx logs
sudo tail -f /var/log/nginx/propvest-api-access.log
sudo tail -f /var/log/nginx/propvest-api-error.log

# Caddy logs
sudo journalctl -u caddy -f
tail -f /var/log/caddy/propvest-api.log
```

### Health Checks

```bash
# Local health check
curl http://localhost:8081/health

# External health check (with SSL)
curl https://api.propvest.com/health
```

### System Resources

```bash
# CPU and memory usage
htop

# Disk usage
df -h

# Check service status
sudo systemctl status propvest-api
sudo systemctl status nginx  # or caddy
sudo systemctl status postgresql
sudo systemctl status redis
```

---

## 🔐 Environment Variables for Production

Create `/opt/propvest/.env`:

```env
# Application
APP_ENV=production
PORT=8081
BASE_URL=https://api.propvest.com

# Database (use production credentials!)
DATABASE_URL=postgres://propvest_user:STRONG_PASSWORD@localhost:5432/propvest?sslmode=require

# Redis
REDIS_URL=redis://:REDIS_PASSWORD@localhost:6379/0

# JWT Secrets (already generated - keep these!)
JWT_SECRET=kYoJ1h0nkA+xhNk1T+ulHTzrXvkn/6c0d1orCvbZ3v62QHT65LBXfpACjws7ipVo
JWT_REFRESH_SECRET=rNIXWGt31mNafYk1/npeTSan7q1wFWx6ZQLEl9POl3NkqZLhZm3j106XgfRMji93
ACCESS_TOKEN_TTL=15m
REFRESH_TOKEN_TTL=720h
BCRYPT_COST=12

# CORS (production frontend URLs)
ALLOWED_ORIGINS=https://propvest.com,https://www.propvest.com,https://app.propvest.com

# Email (production SMTP)
EMAIL_PROVIDER=brevo
BREVO_SMTP_HOST=smtp-relay.brevo.com
BREVO_SMTP_PORT=587
BREVO_SMTP_USERNAME=your-username
BREVO_SMTP_PASSWORD=your-password
SMTP_FROM_EMAIL=noreply@propvest.com
SMTP_FROM_NAME=PropVest

# SMS (production)
SMS_PROVIDER=africastalking
AFRICASTALKING_USERNAME=your-username
AFRICASTALKING_API_KEY=your-api-key
AFRICASTALKING_SENDER_ID=PROPVEST

# Payment (Paystack LIVE keys)
PAYMENT_PROVIDER=paystack
PAYSTACK_SECRET_KEY=sk_live_YOUR_LIVE_SECRET_KEY
PAYSTACK_PUBLIC_KEY=pk_live_YOUR_LIVE_PUBLIC_KEY
PAYSTACK_WEBHOOK_SECRET=your_webhook_secret

# Cloudinary
CLOUDINARY_CLOUD_NAME=your-cloud-name
CLOUDINARY_API_KEY=your-api-key
CLOUDINARY_API_SECRET=your-api-secret

# RabbitMQ (if using)
RABBITMQ_URL=amqp://user:password@localhost:5672/
```

**Security:** Set restrictive permissions:
```bash
sudo chmod 600 /opt/propvest/.env
sudo chown propvest:propvest /opt/propvest/.env
```

---

## 🧪 Testing Production Setup

### 1. SSL/TLS Test

Test SSL configuration:
- **SSL Labs:** https://www.ssllabs.com/ssltest/
- Should get A or A+ rating

### 2. Security Headers Test

Test security headers:
- **Security Headers:** https://securityheaders.com/
- Should get A or A+ rating

### 3. API Functionality Test

```bash
# Health check
curl https://api.propvest.com/health

# Register (should work)
curl -X POST https://api.propvest.com/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "full_name": "Test User",
    "email": "test@example.com",
    "password": "SecurePass123!",
    "phone": "+2348012345678"
  }'

# Rate limiting test (should block after 10 requests)
for i in {1..15}; do
  curl -X POST https://api.propvest.com/api/v1/auth/login \
    -H "Content-Type: application/json" \
    -d '{"email":"test@test.com","password":"wrong"}'
  echo ""
done
```

### 4. Frontend Integration Test

Update frontend `.env`:
```env
VITE_API_URL=https://api.propvest.com
```

Test login/register flows from your React app.

---

## 📈 Performance Optimization

### Database Connection Pooling

In production `.env`:
```env
DB_MAX_OPEN_CONNS=50
DB_MAX_IDLE_CONNS=20
DB_CONN_MAX_LIFETIME=1h
DB_CONN_MAX_IDLE_TIME=15m
```

### Redis for Rate Limiting (Recommended)

For multiple backend instances, upgrade to Redis-based rate limiting:

```go
// In internal/middleware/rate_limit.go
// Replace in-memory implementation with:
import "github.com/go-redis/redis_rate"

limiter := redis_rate.NewLimiter(redisClient)
res, err := limiter.Allow(ctx, key, redis_rate.PerMinute(60))
```

### Load Balancing

If running multiple instances, configure in Nginx:

```nginx
upstream propvest_backend {
    server 127.0.0.1:8081;
    server 127.0.0.1:8082;
    server 127.0.0.1:8083;
    keepalive 32;
}
```

Or in Caddy:

```caddyfile
api.propvest.com {
    reverse_proxy {
        to localhost:8081
        to localhost:8082
        to localhost:8083
        lb_policy round_robin
    }
}
```

---

## 🔄 Updates & Maintenance

### Deploying Updates

```bash
# On development machine
GOOS=linux GOARCH=amd64 go build -o propvest-api cmd/api/main.go

# Copy to server
scp propvest-api user@your-server:/opt/propvest/propvest-api.new

# On server
ssh user@your-server
sudo systemctl stop propvest-api
sudo mv /opt/propvest/propvest-api.new /opt/propvest/propvest-api
sudo chmod +x /opt/propvest/propvest-api
sudo systemctl start propvest-api
```

### Database Migrations

```bash
# Run migrations on production
cd /opt/propvest
./propvest-api migrate-up  # If you add this command

# Or use migrate CLI
migrate -path internal/database/migrations \
        -database "$DATABASE_URL" up
```

### Backup Database

```bash
# Create backup
pg_dump -U propvest_user propvest > backup-$(date +%Y%m%d).sql

# Restore from backup
psql -U propvest_user propvest < backup-20240115.sql
```

---

## 🆘 Troubleshooting

### Service Won't Start

```bash
# Check logs
sudo journalctl -u propvest-api -n 50

# Check if port is in use
sudo netstat -tlnp | grep 8081

# Check environment
sudo -u propvest /opt/propvest/propvest-api
```

### SSL Certificate Issues

```bash
# Renew certificate manually
sudo certbot renew

# Check certificate expiry
echo | openssl s_client -connect api.propvest.com:443 2>/dev/null | openssl x509 -noout -dates
```

### High CPU/Memory Usage

```bash
# Check resource usage
htop

# Check Go process
ps aux | grep propvest-api

# Check database connections
sudo -u postgres psql -c "SELECT count(*) FROM pg_stat_activity;"
```

### Rate Limiting Too Aggressive

Edit `/opt/propvest/.env` or adjust in `cmd/api/main.go`:
```go
r.Use(middleware.RateLimit(middleware.RateLimitConfig{
    AnonymousLimit:     200,  // Increase from 100
    AuthenticatedLimit: 500,  // Increase from 300
    AuthEndpointLimit:  20,   // Increase from 10
}))
```

---

## ✅ Production Checklist

- [ ] Strong JWT secrets generated and set
- [ ] APP_ENV=production
- [ ] Production database configured
- [ ] Production Redis configured
- [ ] SSL/TLS certificate obtained
- [ ] Reverse proxy configured (Nginx/Caddy)
- [ ] Firewall configured
- [ ] CORS updated for production origins
- [ ] Rate limiting enabled
- [ ] Security headers enabled
- [ ] Monitoring set up
- [ ] Backup system configured
- [ ] Log rotation configured
- [ ] Error tracking set up (Sentry)
- [ ] Health checks configured
- [ ] Load balancing (if needed)
- [ ] CI/CD pipeline (optional)
- [ ] Documentation updated
- [ ] Team trained on deployment process

---

## 📚 Additional Resources

- **Let's Encrypt:** https://letsencrypt.org/
- **Nginx Docs:** https://nginx.org/en/docs/
- **Caddy Docs:** https://caddyserver.com/docs/
- **SSL Labs Test:** https://www.ssllabs.com/ssltest/
- **Security Headers Test:** https://securityheaders.com/
- **OWASP Security:** https://owasp.org/

---

**Last Updated:** January 2024  
**Status:** ✅ Ready for Production Deployment
