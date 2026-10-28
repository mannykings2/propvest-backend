# Notification System - Deployment Checklist

**Project:** PropVest Backend  
**Module:** Notification System (Milestone 6)  
**Version:** 1.0  
**Date:** 2026-10-06  

---

## Pre-Deployment Checklist

### 1. Database Preparation ✅

- [ ] **Backup Current Database**
  ```bash
  pg_dump propvest_db > backup_pre_notification_$(date +%Y%m%d).sql
  ```

- [ ] **Verify Migration Files Exist**
  - [ ] `internal/database/migrations/000022_create_notifications.up.sql`
  - [ ] `internal/database/migrations/000022_create_notifications.down.sql`

- [ ] **Test Migration on Development**
  ```bash
  make migrate-up
  make migrate-version  # Should show: 22
  ```

- [ ] **Test Rollback**
  ```bash
  make migrate-down
  make migrate-up
  ```

### 2. Code Compilation ✅

- [ ] **Build API Successfully**
  ```bash
  go build ./cmd/api
  # Should exit with code 0
  ```

- [ ] **Build Worker Successfully**
  ```bash
  go build ./cmd/worker
  # Should exit with code 0
  ```

- [ ] **Run Go Tests**
  ```bash
  go test ./...
  # All tests should pass
  ```

### 3. Configuration Verification

- [ ] **Environment Variables Set**
  ```bash
  # Required for notification system
  RABBITMQ_URL=amqp://guest:guest@localhost:5672/
  SMTP_HOST=smtp.gmail.com
  SMTP_PORT=587
  SMTP_USER=your-email@gmail.com
  SMTP_PASSWORD=your-app-password
  SMS_PROVIDER=twilio  # or africa_talking
  SMS_API_KEY=your-sms-api-key
  ```

- [ ] **RabbitMQ Connection**
  ```bash
  # Test RabbitMQ is accessible
  curl -u guest:guest http://localhost:15672/api/overview
  ```

- [ ] **SMTP Configuration**
  - [ ] Valid SMTP credentials
  - [ ] Test email send (optional, can skip if using dev mode)

- [ ] **SMS Provider Configuration**
  - [ ] Valid API credentials
  - [ ] Test SMS send (optional, can skip if using dev mode)

### 4. Dependencies Check

- [ ] **Go Modules Updated**
  ```bash
  go mod tidy
  go mod verify
  ```

- [ ] **No Compilation Warnings**
  ```bash
  go vet ./...
  ```

- [ ] **Code Formatted**
  ```bash
  go fmt ./...
  ```

---

## Deployment Steps

### Staging Deployment

#### Step 1: Database Migration

```bash
# 1. SSH into staging server
ssh user@staging.propvest.com

# 2. Navigate to project directory
cd /var/www/propvest-backend

# 3. Pull latest code
git pull origin main

# 4. Backup database
pg_dump propvest_staging > backup_$(date +%Y%m%d_%H%M%S).sql

# 5. Run migration
make migrate-up

# 6. Verify migration
make migrate-version
# Expected: 22

# 7. Verify tables created
psql propvest_staging -c "\d notifications"
psql propvest_staging -c "\d notification_preferences"
```

#### Step 2: Build & Deploy

```bash
# 1. Build binaries
go build -o api ./cmd/api
go build -o worker ./cmd/worker

# 2. Stop existing services
sudo systemctl stop propvest-api
sudo systemctl stop propvest-worker

# 3. Replace binaries
sudo cp api /usr/local/bin/propvest-api
sudo cp worker /usr/local/bin/propvest-worker

# 4. Start services
sudo systemctl start propvest-api
sudo systemctl start propvest-worker

# 5. Check service status
sudo systemctl status propvest-api
sudo systemctl status propvest-worker
```

#### Step 3: Verify Deployment

```bash
# 1. Check API health
curl http://staging.propvest.com/api/v1/health

# 2. Check logs
sudo journalctl -u propvest-api -f
sudo journalctl -u propvest-worker -f

# 3. Test notification endpoint
curl -X GET http://staging.propvest.com/api/v1/notifications \
  -H "Authorization: Bearer YOUR_STAGING_TOKEN"
```

### Production Deployment

#### Pre-Production Checks

- [ ] **All staging tests passed**
- [ ] **Code review completed**
- [ ] **Database backup verified**
- [ ] **Rollback plan prepared**
- [ ] **Monitoring alerts configured**

#### Production Steps

```bash
# 1. Backup production database
pg_dump propvest_production > prod_backup_$(date +%Y%m%d_%H%M%S).sql
gzip prod_backup_*.sql

# 2. Deploy during maintenance window
# (Follow same steps as staging)

# 3. Monitor for errors
tail -f /var/log/propvest/api.log
tail -f /var/log/propvest/worker.log
```

---

## Post-Deployment Verification

### 1. API Endpoint Tests

#### Test User Notification Endpoints

```bash
# Get auth token
TOKEN=$(curl -s -X POST https://api.propvest.com/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@propvest.com","password":"test123"}' \
  | jq -r '.data.access_token')

# Test 1: List notifications
curl -X GET "https://api.propvest.com/api/v1/notifications?page=1&page_size=10" \
  -H "Authorization: Bearer $TOKEN"
# Expected: 200 OK with empty or populated list

# Test 2: Get preferences
curl -X GET "https://api.propvest.com/api/v1/notifications/preferences" \
  -H "Authorization: Bearer $TOKEN"
# Expected: 200 OK with preferences array

# Test 3: Update preference
curl -X PUT "https://api.propvest.com/api/v1/notifications/preferences/wallet_credited" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"email_enabled":false}'
# Expected: 200 OK with updated preference

# Test 4: Mark all as read
curl -X PATCH "https://api.propvest.com/api/v1/notifications/read-all" \
  -H "Authorization: Bearer $TOKEN"
# Expected: 200 OK
```

#### Test Admin Endpoints

```bash
# Get admin token
ADMIN_TOKEN=$(curl -s -X POST https://api.propvest.com/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@propvest.com","password":"admin_password"}' \
  | jq -r '.data.access_token')

# Test: Send bulk message (single user)
curl -X POST "https://api.propvest.com/api/v1/admin/messaging/send" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "message_type": "email",
    "targeting": {
      "mode": "single",
      "user_ids": ["test-user-uuid"]
    },
    "content": {
      "subject": "Test Message",
      "body": "This is a test notification from the admin panel."
    }
  }'
# Expected: 200 OK with bulk message response
```

### 2. Database Verification

```bash
# Connect to database
psql propvest_production

# Check tables exist
\d notifications
\d notification_preferences

# Verify indexes
\di notifications*
\di notification_preferences*

# Check sample data (after some usage)
SELECT COUNT(*) FROM notifications;
SELECT COUNT(*) FROM notification_preferences;

# Verify trigger exists
\df update_notification_preferences_updated_at
```

### 3. Integration Verification

#### Test Wallet Integration

```bash
# Trigger wallet deposit (creates notification)
curl -X POST "https://api.propvest.com/api/v1/wallet/deposit" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"amount": 1000, "payment_method": "paystack"}'

# Complete Paystack payment via UI

# Check for notification
curl -X GET "https://api.propvest.com/api/v1/notifications?unread_only=true" \
  -H "Authorization: Bearer $TOKEN"
# Expected: Should see "Wallet Credited" notification
```

#### Test RabbitMQ Integration

```bash
# Check RabbitMQ queues
curl -u admin:password http://localhost:15672/api/queues | jq '.[] | select(.name | contains("propvest"))'

# Expected queues:
# - propvest.email.dispatch
# - propvest.sms.dispatch
# - propvest.realtime.push

# Check queue message counts
# Email queue should have pending emails if bulk send was triggered
```

#### Test Realtime Consumer

```bash
# Check API logs for consumer startup
sudo journalctl -u propvest-api --since "10 minutes ago" | grep -i "realtime"
# Expected: "Starting realtime consumer" or similar message

# Trigger notification and watch logs
# Should see realtime push events in logs
```

### 4. Performance Verification

```bash
# Test pagination performance
time curl -X GET "https://api.propvest.com/api/v1/notifications?page=1&page_size=100" \
  -H "Authorization: Bearer $TOKEN"
# Expected: < 500ms response time

# Test database query performance
psql propvest_production -c "EXPLAIN ANALYZE SELECT * FROM notifications WHERE user_id = 'uuid' AND read = false ORDER BY created_at DESC LIMIT 20;"
# Expected: Index scan, < 10ms
```

### 5. Error Handling Verification

```bash
# Test invalid notification ID
curl -X PATCH "https://api.propvest.com/api/v1/notifications/invalid-uuid/read" \
  -H "Authorization: Bearer $TOKEN"
# Expected: 400 Bad Request

# Test unauthorized access
curl -X GET "https://api.propvest.com/api/v1/notifications"
# Expected: 401 Unauthorized

# Test admin endpoint without admin role
curl -X POST "https://api.propvest.com/api/v1/admin/messaging/send" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"message_type":"email","targeting":{"mode":"all","confirm_all_users":true},"content":{"subject":"Test","body":"Test message"}}'
# Expected: 403 Forbidden
```

---

## Monitoring Setup

### 1. Application Metrics

```bash
# Monitor notification creation rate
SELECT DATE_TRUNC('hour', created_at) as hour, COUNT(*) as count
FROM notifications
WHERE created_at > NOW() - INTERVAL '24 hours'
GROUP BY hour
ORDER BY hour DESC;

# Monitor unread notifications
SELECT COUNT(*) as total_unread
FROM notifications
WHERE read = false;

# Monitor notification types distribution
SELECT type, COUNT(*) as count
FROM notifications
WHERE created_at > NOW() - INTERVAL '7 days'
GROUP BY type
ORDER BY count DESC;
```

### 2. Queue Metrics

```bash
# Monitor RabbitMQ queue lengths
curl -u admin:password http://localhost:15672/api/queues | \
  jq '.[] | select(.name | contains("propvest")) | {name, messages}'
```

### 3. Error Monitoring

```bash
# Monitor API errors
sudo journalctl -u propvest-api --since "1 hour ago" | grep -i "error"

# Monitor worker errors
sudo journalctl -u propvest-worker --since "1 hour ago" | grep -i "error"
```

### 4. Alert Configuration

Set up alerts for:
- [ ] Notification creation failures
- [ ] RabbitMQ queue depth > 1000
- [ ] Email/SMS send failures > 10/hour
- [ ] Database query time > 1s
- [ ] API response time > 2s

---

## Rollback Procedure

If issues are detected:

### Immediate Rollback

```bash
# 1. Stop services
sudo systemctl stop propvest-api
sudo systemctl stop propvest-worker

# 2. Restore previous binaries
sudo cp /backup/propvest-api /usr/local/bin/propvest-api
sudo cp /backup/propvest-worker /usr/local/bin/propvest-worker

# 3. Rollback database migration
make migrate-down

# 4. Verify migration version
make migrate-version
# Expected: 21 (previous version)

# 5. Restore database from backup (if needed)
psql propvest_production < backup_pre_notification_YYYYMMDD.sql

# 6. Start services
sudo systemctl start propvest-api
sudo systemctl start propvest-worker

# 7. Verify health
curl http://localhost:8080/api/v1/health
```

### Post-Rollback

- [ ] Document issues encountered
- [ ] Update deployment checklist
- [ ] Fix issues in development
- [ ] Re-test in staging
- [ ] Schedule new deployment

---

## Success Criteria

Deployment is considered successful when:

- [x] All API builds compile without errors
- [x] Database migrations applied successfully
- [x] All 8 notification endpoints respond correctly
- [ ] Wallet integration creates notifications
- [ ] Email/SMS queues are processing
- [ ] Realtime consumer is running
- [ ] No critical errors in logs (first 30 minutes)
- [ ] Response times < 500ms for list endpoints
- [ ] Admin bulk messaging works correctly

---

## Post-Deployment Tasks

### Within 24 Hours

- [ ] Monitor error logs for any issues
- [ ] Check notification creation rates
- [ ] Verify email/SMS delivery
- [ ] Review database performance
- [ ] Check RabbitMQ queue processing

### Within 1 Week

- [ ] Collect user feedback on notifications
- [ ] Review notification preferences usage
- [ ] Optimize slow queries (if any)
- [ ] Adjust rate limits if needed
- [ ] Document any issues encountered

### Within 1 Month

- [ ] Add monitoring dashboards
- [ ] Set up automated alerts
- [ ] Review notification effectiveness
- [ ] Plan future enhancements
- [ ] Update documentation based on usage

---

## Support Contacts

**For Deployment Issues:**
- DevOps Team: devops@propvest.com
- On-Call Engineer: +234-XXX-XXX-XXXX

**For Code Issues:**
- Backend Lead: backend@propvest.com
- GitHub Issues: https://github.com/propvest/backend/issues

---

**Checklist Version:** 1.0  
**Last Updated:** 2026-10-06  
**Next Review:** After first production deployment  
