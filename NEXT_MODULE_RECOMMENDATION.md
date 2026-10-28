# What's Next After Investment Module?

**Date:** October 6, 2026  
**Current Status:** Investment Module (Milestone 5) Complete ✅  
**Question:** What module should be built next?

---

## 🎯 Recommended Next Module

### **Milestone 6: Notification System** 🔔

**Priority:** 🟢 MEDIUM  
**Complexity:** MEDIUM  
**Estimated Time:** 2-3 days  
**Status:** ~5% complete (stubs exist)

---

## 📋 Why Notifications Next?

### 1. **Natural Flow** (Following Roadmap)
```
Milestone 0: Foundation ✅
    ↓
Milestone 1: Authentication ✅
    ↓
Milestone 2: User Management ✅
    ↓
Milestone 3: Wallet System ✅
    ↓
Milestone 4: Property Module ✅
    ↓
Milestone 5: Investment Engine ✅
    ↓
→ Milestone 6: Notification System 📋 ← YOU ARE HERE
    ↓
Milestone 7: Administration 📋
    ↓
Milestone 8: Production Hardening 📋
```

### 2. **Already Partially Stubbed**
The codebase has notification infrastructure ready:

✅ **Queue Configuration**
- `NotificationQueueConfig` defined in `internal/queue/config.go`
- Queue name: `propvest.notification.dispatch`
- Concurrency: 20 (high throughput)
- Priority: 1 (can be delayed)

✅ **Service Interface**
- `NotificationService` interface exists
- Currently set to `nil` in `main.go`
- Wallet service already calls it:
  ```go
  s.notifier.Notify(ctx, userID, models.NotificationDepositSuccess, ...)
  ```

✅ **Model Constants**
- `NotificationDepositSuccess`
- `NotificationWithdrawalUpdate`
- Event types defined in outbox

✅ **WebSocket Hub**
- Realtime hub already implemented (`internal/realtime/hub.go`)
- Can push to connected users
- Comment: "the durable copy lives in the notifications table"

✅ **RabbitMQ Integration**
- Worker already set up for async dispatch
- Email/SMS queue consumers ready
- Just needs notification handler

### 3. **Completes the User Experience**
Current gaps that notifications would fill:

❌ **No user feedback** after investment  
❌ **No email confirmations** for transactions  
❌ **No SMS alerts** for critical events  
❌ **No in-app notification history**  
❌ **No notification preferences**  

With notifications:
✅ Investment confirmed → Email + Push  
✅ Property funded → Notify all investors  
✅ Deposit cleared → SMS + In-app  
✅ Withdrawal approved → Email confirmation  

### 4. **Low Complexity** (Infrastructure exists)
What's needed:
- `notifications` table (migration)
- Notification repository (CRUD)
- Notification service (business logic)
- Notification handler (API endpoints)
- Queue consumer (dispatch emails/SMS)
- Templates (email/SMS content)

What's **already done**:
- Queue setup ✅
- WebSocket hub ✅
- Email sender stub ✅
- SMS sender stub ✅
- Service interface ✅

---

## 📦 What Notification Module Includes

### Core Features

#### 1. In-App Notifications
- **Notification Model** (id, user_id, type, title, message, read_at, created_at)
- **Notification Repository** (Create, FindByUser, MarkAsRead, MarkAllAsRead)
- **API Endpoints:**
  - `GET /notifications` - List user's notifications
  - `GET /notifications/:id` - Get single notification
  - `PATCH /notifications/:id/read` - Mark as read
  - `PATCH /notifications/read-all` - Mark all as read
  - `DELETE /notifications/:id` - Delete notification

#### 2. Email Notifications
- **Templates** (deposit, withdrawal, investment, property updates)
- **Queue Consumer** (listens to `propvest.notification.dispatch`)
- **Retry Logic** (failed emails go to DLQ)
- **HTML Templates** (beautiful, branded emails)

#### 3. SMS Notifications
- **Integration** (already stubbed in `internal/utils/sms/sms.go`)
- **Queue Consumer** (same queue, different message type)
- **Templates** (short SMS for critical events)

#### 4. Push Notifications (Realtime)
- **WebSocket** (already implemented in hub)
- **Consumer** (listens to `propvest.realtime.push` queue)
- **Fan-out** (sends to all connected clients of a user)

#### 5. User Preferences
- **Preferences Model** (user_id, email_enabled, sms_enabled, push_enabled)
- **API Endpoints:**
  - `GET /users/me/notification-preferences`
  - `PATCH /users/me/notification-preferences`
- **Respect User Choices** (don't spam if disabled)

---

## 🗄️ Database Changes

### Migration: 000022_create_notifications

```sql
-- Notifications table
CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,  -- deposit, withdrawal, investment, property, system
    title VARCHAR(255) NOT NULL,
    message TEXT NOT NULL,
    data JSONB,  -- Additional metadata (investment_id, property_id, etc.)
    read_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_user_created ON notifications(user_id, created_at DESC);
CREATE INDEX idx_notifications_user_read ON notifications(user_id, read_at);
CREATE INDEX idx_notifications_type ON notifications(type);

-- Notification preferences table
CREATE TABLE notification_preferences (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    email_enabled BOOLEAN DEFAULT TRUE,
    sms_enabled BOOLEAN DEFAULT TRUE,
    push_enabled BOOLEAN DEFAULT TRUE,
    email_deposit BOOLEAN DEFAULT TRUE,
    email_withdrawal BOOLEAN DEFAULT TRUE,
    email_investment BOOLEAN DEFAULT TRUE,
    email_property BOOLEAN DEFAULT TRUE,
    sms_deposit BOOLEAN DEFAULT TRUE,
    sms_withdrawal BOOLEAN DEFAULT TRUE,
    sms_investment BOOLEAN DEFAULT FALSE,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

---

## 🏗️ Implementation Plan

### Phase 1: Database & Models (2-3 hours)
1. Create migration `000022_create_notifications`
2. Create `Notification` model
3. Create `NotificationPreference` model
4. Run migration

### Phase 2: Repository Layer (2-3 hours)
1. `NotificationRepository` interface
2. Implement CRUD methods
3. Implement queries (FindByUser, CountUnread)
4. Unit tests

### Phase 3: Service Layer (4-5 hours)
1. `NotificationService` implementation
2. Create notification (in-app)
3. Send email (queue dispatch)
4. Send SMS (queue dispatch)
5. Send push (WebSocket)
6. Respect user preferences
7. Unit tests

### Phase 4: Handler Layer (2-3 hours)
1. `NotificationHandler`
2. List notifications
3. Mark as read
4. Preferences endpoints
5. Integration tests

### Phase 5: Queue Consumers (3-4 hours)
1. Email consumer (listens to notification queue)
2. SMS consumer (same queue, different type)
3. Realtime consumer (WebSocket fan-out)
4. Error handling & retries

### Phase 6: Templates (2-3 hours)
1. Email HTML templates (deposit, withdrawal, investment)
2. SMS templates (short messages)
3. Template testing

### Phase 7: Integration (1-2 hours)
1. Wire up in `main.go`
2. Enable notification calls in wallet/investment services
3. Test end-to-end
4. Documentation

**Total Estimated Time:** 16-23 hours (~2-3 days)

---

## 🔄 Alternative: Skip to Administration

### If you want to skip notifications:

**Milestone 7: Administration** could be next, but:

❌ **Less polished UX** - Users won't get feedback  
❌ **Harder to test** - No notifications to verify workflows  
❌ **Missing infrastructure** - Admin actions need notifications  
⚠️ **Higher complexity** - Admin is larger (dashboard, audit logs, reports)  

**Recommendation:** Do notifications first. It's easier, improves UX immediately, and admin features will benefit from notifications (e.g., "Admin approved your property" notification).

---

## 🎯 Decision Matrix

| Factor | Notifications (M6) | Administration (M7) |
|--------|-------------------|---------------------|
| **Complexity** | 🟢 Medium | 🟡 High |
| **Time** | 2-3 days | 4-5 days |
| **User Impact** | 🟢 High (immediate value) | 🟡 Medium (admin only) |
| **Dependencies** | 🟢 Infrastructure ready | 🟡 Needs notifications |
| **Roadmap Order** | ✅ Next in sequence | ⏭️ Skip ahead |
| **Code Readiness** | ✅ 5% stubbed | ❌ 0% started |

**Winner:** 🔔 **Notifications (Milestone 6)**

---

## 📊 Current Overall Progress

```
Milestone 0: Foundation           ✅ 100%
Milestone 1: Authentication       ✅ 100%
Milestone 2: User Management      ✅ 100%
Milestone 3: Wallet System        ✅ 100%
Milestone 4: Property Module      ✅ 100%
Milestone 5: Investment Engine    ✅ 100%  ← YOU ARE HERE
─────────────────────────────────────────
Milestone 6: Notification System  📋 5%   ← NEXT
Milestone 7: Administration       📋 0%
Milestone 8: Production Hardening 📋 0%
─────────────────────────────────────────
Overall Backend: ~67% Complete
```

---

## ✅ Recommendation

### **Build Milestone 6: Notification System Next**

**Reasons:**
1. ✅ Follows the official roadmap sequence
2. ✅ Infrastructure already ~5% in place
3. ✅ Medium complexity (achievable in 2-3 days)
4. ✅ High user impact (completes the feedback loop)
5. ✅ Admin module will benefit from it
6. ✅ Testing will be easier with notifications

**Next Steps:**
1. Create `NOTIFICATION_MODULE_TASKS.md` (like Investment Module)
2. Start with Phase 1: Database migration
3. Follow the 7-phase implementation plan above
4. Test with existing features (wallet deposits, investments)

---

## 🚀 Quick Start Command

Ready to proceed? Say:

```
"Proceed with Milestone 6: Notification System"
```

Or if you want a detailed task breakdown first:

```
"Generate a tasks file for Notification Module (Milestone 6)"
```

---

**Status:** 📋 **Ready to Start**  
**Module:** Notification System (Milestone 6)  
**Priority:** Next in sequence  
**Estimated Time:** 2-3 days (16-23 hours)

---

*Recommendation prepared based on:*
- Official roadmap (`8.1-BACKEND_IMPLEMENTATION_ROADMAP.md`)
- Current codebase analysis (notifications ~5% stubbed)
- Dependency graph (M6 before M7)
- Implementation complexity assessment
