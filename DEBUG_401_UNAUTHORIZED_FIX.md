# Debug Case Study: 401 Unauthorized After Successful Login

## 🐛 The Problem

Users could successfully login (status 200) but immediately received 401 Unauthorized when accessing protected endpoints like `/api/v1/users/me`.

## 🔍 Debug Process (How to Analyze Similar Issues)

### Step 1: Read the Logs Carefully

```
✅ POST /api/v1/auth/login status=200 duration_ms=568
❌ GET /api/v1/users/me status=401 duration_ms=5
```

**Key Observation**: Login succeeded, but the very next authenticated request failed. This suggests a problem with how tokens are being validated, not with the token itself.

### Step 2: Form Hypotheses

When authentication works but authorization fails, check:

1. ❓ Is the token being sent correctly? (Header format)
2. ❓ Is the middleware extracting the token correctly?
3. ❓ Is there a type mismatch between what middleware stores and what handlers expect?
4. ❓ Are the secrets matching (JWT_SECRET)?

### Step 3: Trace the Data Flow

**Authentication Flow:**
```
Client Request
  ↓
1. Middleware extracts token from "Authorization: Bearer <token>"
  ↓
2. Middleware validates token with jwt.ValidateToken()
  ↓
3. Middleware stores user info: c.Set("user_id", ???)
  ↓
4. Handler retrieves user info: getUserIDFromContext()
  ↓
5. Handler uses user_id for business logic
```

**Question**: What type is stored in step 3, and what type is expected in step 4?

### Step 4: Examine the Code

**File 1: `internal/middleware/auth.go` (Line 98-99)**
```go
c.Set("user_id", claims.UserID.String())  // 🔴 Stores as STRING
c.Set("role", claims.Role)
```

**File 2: `internal/handlers/helpers.go` (Line 26-29)**
```go
userID, ok := userIDStr.(uuid.UUID)  // 🔴 Expects uuid.UUID TYPE
if !ok {
    return uuid.Nil, errors.ErrUnauthorized  // ❌ Fails here!
}
```

### Step 5: Identify Root Cause

**The Bug: Type Mismatch**

- **Stored**: `string` (e.g., "d9bfa8b2-63c9-46ef-8ff9-ea51286a25bf")
- **Expected**: `uuid.UUID` (a struct type)
- **Result**: Type assertion fails → 401 Unauthorized

**Why This Happened:**

The middleware was changed at some point to store `claims.UserID.String()` (string) instead of `claims.UserID` (uuid.UUID), but the helper function wasn't updated to match.

## ✅ The Solution

Update `getUserIDFromContext()` to parse the string:

**Before:**
```go
userID, ok := userIDStr.(uuid.UUID)  // Wrong type expected
if !ok {
    return uuid.Nil, errors.ErrUnauthorized
}
```

**After:**
```go
// Step 2: Type assertion to string (middleware stores it as string)
userIDStr, ok := userIDValue.(string)
if !ok {
    return uuid.Nil, errors.ErrUnauthorized
}

// Step 3: Parse string to UUID
userID, err := uuid.Parse(userIDStr)
if err != nil {
    return uuid.Nil, errors.ErrUnauthorized
}
```

## 🎓 Key Lessons for Debugging

### 1. **Read Logs Top-to-Bottom**
Don't just look at errors. The success logs tell you what's working, which narrows down where the problem is.

### 2. **Trace Data Flow**
Follow the data from input to output:
- Where is it created?
- Where is it stored?
- Where is it retrieved?
- Where is it used?

### 3. **Check Type Consistency**
When data crosses boundaries (network, storage, context), verify:
- What type goes IN?
- What type comes OUT?
- Do they match?

### 4. **Look for Recent Changes**
Type mismatches often appear after refactoring. Check git history:
```bash
git log --oneline -- internal/middleware/auth.go
git log --oneline -- internal/handlers/helpers.go
```

### 5. **Add Strategic Logging**
When debugging, add logs at boundaries:
```go
// In middleware
fmt.Printf("DEBUG: Storing user_id as type: %T value: %v\n", claims.UserID.String(), claims.UserID.String())

// In helper
fmt.Printf("DEBUG: Retrieved user_id as type: %T value: %v\n", userIDValue, userIDValue)
```

## 🛠️ Prevention

### Code Review Checklist

When changing how data is stored in context:

- [ ] Check all places that retrieve this data
- [ ] Verify type consistency
- [ ] Add type assertions with clear error messages
- [ ] Document the expected type in comments
- [ ] Write integration tests that cover the full flow

### Type Safety Tips

1. **Document types explicitly:**
   ```go
   // SetUserID stores the user ID in context as a string.
   // Handlers must use uuid.Parse() to convert it.
   c.Set("user_id", userID.String())
   ```

2. **Use helper functions:**
   ```go
   // GetUserID safely extracts and parses the user ID
   func GetUserID(c *gin.Context) (uuid.UUID, error) {
       // Implementation handles type conversion
   }
   ```

3. **Write tests:**
   ```go
   func TestGetUserIDFromContext(t *testing.T) {
       c, _ := gin.CreateTestContext(nil)
       c.Set("user_id", "d9bfa8b2-63c9-46ef-8ff9-ea51286a25bf")
       
       userID, err := getUserIDFromContext(c)
       assert.NoError(t, err)
       assert.NotEqual(t, uuid.Nil, userID)
   }
   ```

## 📊 Impact

**Before Fix:**
- All authenticated endpoints returned 401
- Users couldn't access their profiles, wallets, etc.
- System appeared completely broken after login

**After Fix:**
- All authenticated endpoints work correctly
- Users can access protected resources
- Token-based auth flow is fully functional

## 🔗 Related Files

- `internal/middleware/auth.go` - Sets user_id in context
- `internal/handlers/helpers.go` - Retrieves user_id from context
- `internal/utils/jwt/jwt.go` - Token validation logic
- All authenticated handlers - Depend on getUserIDFromContext()

---

**Date Fixed**: 2026-08-10  
**Fixed By**: AI Assistant  
**Severity**: Critical (blocked all authenticated endpoints)  
**Time to Debug**: ~10 minutes using systematic approach
