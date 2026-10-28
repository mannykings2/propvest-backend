# Notification System - API Reference

**Version:** 1.0  
**Base URL:** `http://localhost:8080/api/v1` (development)  
**Authentication:** JWT Bearer Token (required for all endpoints)  

---

## Table of Contents

1. [Authentication](#authentication)
2. [User Notification Endpoints](#user-notification-endpoints)
3. [Notification Preference Endpoints](#notification-preference-endpoints)
4. [Admin Bulk Messaging Endpoints](#admin-bulk-messaging-endpoints)
5. [Error Responses](#error-responses)
6. [Rate Limiting](#rate-limiting)

---

## Authentication

All endpoints require a valid JWT token in the Authorization header:

```
Authorization: Bearer <your_jwt_token>
```

To obtain a token:
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "your_password"
  }'
```

---

## User Notification Endpoints

### 1. List Notifications

Retrieve paginated list of user's notifications.

**Endpoint:** `GET /notifications`  
**Auth:** Required  
**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| page | integer | 1 | Page number (min: 1) |
| page_size | integer | 20 | Items per page (max: 100) |
| unread_only | boolean | false | Filter unread notifications |

**Example Request:**
```bash
curl -X GET "http://localhost:8080/api/v1/notifications?page=1&page_size=10&unread_only=true" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Success Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "notifications": [
      {
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "type": "wallet_credited",
        "title": "Wallet Credited",
        "body": "Your wallet has been credited with ₦50,000",
        "read": false,
        "metadata": {
          "transaction_id": "TXN123",
          "amount": 50000
        },
        "created_at": "2026-10-06T10:30:00Z"
      }
    ],
    "unread_count": 5,
    "pagination": {
      "page": 1,
      "page_size": 10,
      "total_items": 25,
      "total_pages": 3
    }
  }
}
```

---

### 2. Mark Notification as Read

Mark a single notification as read.

**Endpoint:** `PATCH /notifications/:id/read`  
**Auth:** Required  
**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| id | UUID | Notification ID |

**Example Request:**
```bash
curl -X PATCH "http://localhost:8080/api/v1/notifications/550e8400-e29b-41d4-a716-446655440000/read" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Success Response (200 OK):**
```json
{
  "success": true,
  "message": "Notification marked as read"
}
```

**Error Responses:**
- `400 Bad Request` - Invalid notification ID format
- `404 Not Found` - Notification not found or not owned by user

---

### 3. Mark All Notifications as Read

Mark all user's unread notifications as read.

**Endpoint:** `PATCH /notifications/read-all`  
**Auth:** Required  

**Example Request:**
```bash
curl -X PATCH "http://localhost:8080/api/v1/notifications/read-all" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Success Response (200 OK):**
```json
{
  "success": true,
  "message": "All notifications marked as read"
}
```

---

### 4. Delete Notification

Delete a notification from user's notification center.

**Endpoint:** `DELETE /notifications/:id`  
**Auth:** Required  
**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| id | UUID | Notification ID |

**Example Request:**
```bash
curl -X DELETE "http://localhost:8080/api/v1/notifications/550e8400-e29b-41d4-a716-446655440000" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Success Response (200 OK):**
```json
{
  "success": true,
  "message": "Notification deleted successfully"
}
```

**Error Responses:**
- `400 Bad Request` - Invalid notification ID format
- `404 Not Found` - Notification not found or not owned by user

---

## Notification Preference Endpoints

### 5. Get Notification Preferences

Retrieve all notification preferences for the authenticated user.

**Endpoint:** `GET /notifications/preferences`  
**Auth:** Required  

**Example Request:**
```bash
curl -X GET "http://localhost:8080/api/v1/notifications/preferences" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Success Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "preferences": [
      {
        "notification_type": "wallet_credited",
        "in_app_enabled": true,
        "email_enabled": true,
        "sms_enabled": false
      },
      {
        "notification_type": "investment_confirmed",
        "in_app_enabled": true,
        "email_enabled": true,
        "sms_enabled": false
      }
    ]
  }
}
```

---

### 6. Update Type-Specific Preference

Update notification preferences for a specific notification type.

**Endpoint:** `PUT /notifications/preferences/:type`  
**Auth:** Required  
**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| type | string | Notification type (e.g., "wallet_credited") |

**Request Body:**
```json
{
  "in_app_enabled": true,
  "email_enabled": false,
  "sms_enabled": true
}
```

**Field Notes:**
- All fields are optional (partial updates supported)
- `null` values are ignored
- At least one channel must remain enabled

**Example Request:**
```bash
curl -X PUT "http://localhost:8080/api/v1/notifications/preferences/wallet_credited" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "in_app_enabled": true,
    "email_enabled": false,
    "sms_enabled": true
  }'
```

**Success Response (200 OK):**
```json
{
  "success": true,
  "message": "Preference updated successfully",
  "data": {
    "notification_type": "wallet_credited",
    "in_app_enabled": true,
    "email_enabled": false,
    "sms_enabled": true
  }
}
```

**Error Responses:**
- `400 Bad Request` - Validation failed (e.g., all channels disabled)

---

### 7. Batch Update Preferences

Update multiple notification preferences in a single request.

**Endpoint:** `PUT /notifications/preferences`  
**Auth:** Required  

**Request Body:**
```json
{
  "preferences": [
    {
      "notification_type": "wallet_credited",
      "in_app_enabled": true,
      "email_enabled": false,
      "sms_enabled": true
    },
    {
      "notification_type": "investment_confirmed",
      "in_app_enabled": true,
      "email_enabled": true,
      "sms_enabled": false
    }
  ]
}
```

**Example Request:**
```bash
curl -X PUT "http://localhost:8080/api/v1/notifications/preferences" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "preferences": [
      {
        "notification_type": "wallet_credited",
        "email_enabled": false
      },
      {
        "notification_type": "investment_confirmed",
        "sms_enabled": true
      }
    ]
  }'
```

**Success Response (200 OK):**
```json
{
  "success": true,
  "message": "Preferences updated successfully",
  "data": {
    "preferences": [
      {
        "notification_type": "wallet_credited",
        "in_app_enabled": true,
        "email_enabled": false,
        "sms_enabled": false
      },
      {
        "notification_type": "investment_confirmed",
        "in_app_enabled": true,
        "email_enabled": true,
        "sms_enabled": true
      }
    ]
  }
}
```

**Notes:**
- Processes all preferences sequentially
- Fails on first error (all-or-nothing)
- Returns updated state for all preferences

---

## Admin Bulk Messaging Endpoints

### 8. Send Bulk Message

Send bulk email/SMS messages to targeted users. Requires admin role.

**Endpoint:** `POST /admin/messaging/send`  
**Auth:** Required (Admin role)  

**Request Body:**
```json
{
  "message_type": "both",
  "targeting": {
    "mode": "segment",
    "filters": {
      "kyc_status": "approved",
      "has_investments": true
    }
  },
  "content": {
    "subject": "New Investment Opportunity",
    "body": "Dear investor, we have a new property available for investment. Check it out on the platform!"
  }
}
```

**Field Descriptions:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| message_type | string | Yes | "email", "sms", or "both" |
| targeting.mode | string | Yes | "single", "segment", or "all" |
| targeting.user_ids | array | For "single" | Array of user IDs (max 10,000) |
| targeting.filters | object | For "segment" | User filters (kyc_status, has_investments) |
| targeting.confirm_all_users | boolean | For "all" | Must be `true` for "all" mode |
| content.subject | string | For email | Email subject (max 200 chars) |
| content.body | string | Yes | Message body (10-5000 chars) |

**Targeting Modes:**

1. **Single** - Target specific users by ID:
```json
{
  "targeting": {
    "mode": "single",
    "user_ids": ["uuid1", "uuid2", "uuid3"]
  }
}
```

2. **Segment** - Target users matching filters:
```json
{
  "targeting": {
    "mode": "segment",
    "filters": {
      "kyc_status": "approved",
      "has_investments": true
    }
  }
}
```

3. **All** - Target all users (requires confirmation):
```json
{
  "targeting": {
    "mode": "all",
    "confirm_all_users": true
  }
}
```

**Example Request:**
```bash
curl -X POST "http://localhost:8080/api/v1/admin/messaging/send" \
  -H "Authorization: Bearer ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "message_type": "both",
    "targeting": {
      "mode": "segment",
      "filters": {
        "kyc_status": "approved",
        "has_investments": true
      }
    },
    "content": {
      "subject": "Q4 Investment Update",
      "body": "Dear valued investor, here is your quarterly investment update..."
    }
  }'
```

**Success Response (200 OK):**
```json
{
  "success": true,
  "message": "Bulk message sent successfully",
  "data": {
    "message_id": "550e8400-e29b-41d4-a716-446655440000",
    "targets_count": 150,
    "email_queued": 150,
    "sms_queued": 150,
    "in_app_created": 150
  }
}
```

**Error Responses:**
- `400 Bad Request` - Validation failed
- `403 Forbidden` - Not an admin
- `422 Unprocessable Entity` - No targets matched or limit exceeded (10,000 max)

**Safety Notes:**
- Maximum 10,000 users per broadcast
- "All users" mode requires explicit confirmation
- In-app notifications are created for tracking
- Email/SMS are queued asynchronously

---

## Error Responses

All error responses follow this format:

```json
{
  "success": false,
  "error": "Human-readable error message",
  "code": "error_code"
}
```

**Common Error Codes:**

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `unauthorized` | 401 | Missing or invalid JWT token |
| `forbidden` | 403 | Insufficient permissions |
| `validation_error` | 400 | Request validation failed |
| `not_found` | 404 | Resource not found |
| `internal_error` | 500 | Server error |

---

## Rate Limiting

**Global Limits:**
- Anonymous: 100 requests/minute
- Authenticated: 300 requests/minute

**Endpoint-Specific Limits:**
- Admin bulk messaging: Recommended 10 requests/hour (not enforced yet)

**Rate Limit Headers:**
```
X-RateLimit-Limit: 300
X-RateLimit-Remaining: 299
X-RateLimit-Reset: 1696593600
```

---

## Notification Types

The system supports the following notification types:

**Authentication & Account:**
- `auth_login_new_device`
- `auth_password_changed`
- `account_kyc_approved`
- `account_kyc_rejected`

**Wallet:**
- `wallet_credited`
- `wallet_debited`
- `withdrawal_completed`
- `withdrawal_failed`

**Investment:**
- `investment_confirmed`
- `investment_returns_paid`
- `property_funded`

**Property:**
- `property_published`
- `property_status_changed`

**Admin:**
- `admin_broadcast`
- `system_maintenance`

---

## Pagination

All list endpoints support pagination with these parameters:

| Parameter | Default | Max | Description |
|-----------|---------|-----|-------------|
| page | 1 | - | Page number |
| page_size | 20 | 100 | Items per page |

**Pagination Response Format:**
```json
{
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total_items": 150,
    "total_pages": 8
  }
}
```

---

## Webhooks & Realtime

The notification system supports real-time push notifications via WebSocket.

**WebSocket Connection:**
- Endpoint: `ws://localhost:8080/ws` (not implemented yet)
- Authentication: JWT token in query parameter or header
- Protocol: JSON messages

**Realtime Events:**
```json
{
  "event": "notification.created",
  "payload": {
    "id": "uuid",
    "type": "wallet_credited",
    "title": "Wallet Credited",
    "body": "Your wallet has been credited",
    "read": false,
    "created_at": "2026-10-06T10:30:00Z"
  }
}
```

---

## Testing with cURL

### Complete Testing Flow

```bash
# 1. Login
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password"}' \
  | jq -r '.data.access_token')

# 2. List notifications
curl -X GET "http://localhost:8080/api/v1/notifications?page=1&page_size=10" \
  -H "Authorization: Bearer $TOKEN"

# 3. Mark notification as read
curl -X PATCH "http://localhost:8080/api/v1/notifications/NOTIFICATION_ID/read" \
  -H "Authorization: Bearer $TOKEN"

# 4. Get preferences
curl -X GET "http://localhost:8080/api/v1/notifications/preferences" \
  -H "Authorization: Bearer $TOKEN"

# 5. Update preference
curl -X PUT "http://localhost:8080/api/v1/notifications/preferences/wallet_credited" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"email_enabled":false,"sms_enabled":true}'
```

---

## Support & Contact

For API support or bug reports:
- Documentation: `/docs`
- Issues: Create a GitHub issue
- Email: support@propvest.com

---

**Last Updated:** 2026-10-06  
**API Version:** 1.0  
