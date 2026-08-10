# Avatar Upload - Postman Testing Guide

## Quick Start

### 1. Get Your Access Token First
```
POST http://localhost:8081/api/v1/auth/login

Body (raw JSON):
{
  "email": "your-email@example.com",
  "password": "your-password"
}

Response → Copy the "access_token" value
```

### 2. Upload Avatar
```
PATCH http://localhost:8081/api/v1/users/avatar

Authorization:
  Type: Bearer Token
  Token: <paste your access_token here>

Body:
  Type: form-data
  Key: avatar (set to FILE type)
  Value: Select your image file
```

---

## Detailed Step-by-Step Instructions

### Step 1: Open Postman
- Launch Postman application
- Create a new request

### Step 2: Configure Request Method and URL
1. Change method dropdown to **PATCH**
2. Enter URL: `http://localhost:8081/api/v1/users/avatar`

### Step 3: Add Authentication
1. Click **"Authorization"** tab
2. **Type**: Select "Bearer Token"
3. **Token**: Paste your access token (without "Bearer " prefix)

### Step 4: Configure Request Body
1. Click **"Body"** tab
2. Select **"form-data"** radio button
3. In the KEY column:
   - Click the dropdown (shows "Text" by default)
   - Select **"File"**
   - Type the name: `avatar`
4. In the VALUE column:
   - Click **"Select Files"** button
   - Choose your image file

### Step 5: Send Request
- Click blue **"Send"** button
- Wait for response

### Step 6: Verify Success
Check the response for:
- Status: `200 OK`
- `"success": true`
- `"avatar_url"` field with Cloudinary URL

---

## Visual Guide (Text-Based)

```
┌─────────────────────────────────────────────────────────┐
│ PATCH ▼ │ http://localhost:8081/api/v1/users/avatar  │
├─────────────────────────────────────────────────────────┤
│ Params │ Authorization │ Headers │ Body │ Pre-request │
└─────────────────────────────────────────────────────────┘

Authorization Tab:
┌─────────────────────────────────────────────────────────┐
│ TYPE                                                    │
│ Bearer Token ▼                                          │
│                                                         │
│ TOKEN                                                   │
│ eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...               │
└─────────────────────────────────────────────────────────┘

Body Tab:
┌─────────────────────────────────────────────────────────┐
│ ○ none                                                  │
│ ● form-data                                             │
│ ○ x-www-form-urlencoded                                 │
│ ○ raw                                                   │
│ ○ binary                                                │
└─────────────────────────────────────────────────────────┘

┌──────────────┬────────────────────┬──────────────────────┐
│ KEY          │ VALUE              │ DESCRIPTION          │
├──────────────┼────────────────────┼──────────────────────┤
│ File ▼ avatar│ my-photo.jpg       │                      │
│              │ [Select Files]     │                      │
└──────────────┴────────────────────┴──────────────────────┘
```

---

## Expected Responses

### ✅ Success (200 OK)
```json
{
  "success": true,
  "message": "Avatar uploaded successfully",
  "data": {
    "id": "d9bfa8b2-63c9-46ef-8ff9-ea51286a25bf",
    "user_code": "USER001234",
    "full_name": "Chukwuemeka Okafor",
    "email": "chukwuemeka@example.com",
    "phone": "+2348012345678",
    "avatar_url": "https://res.cloudinary.com/mmgxnfud/image/upload/v1723456789/avatars/d9bfa8b2-63c9-46ef-8ff9-ea51286a25bf.jpg",
    "kyc_status": "pending",
    "role": "investor",
    "created_at": "2026-08-10T05:08:06Z"
  }
}
```

### ❌ No File (400 Bad Request)
```json
{
  "success": false,
  "error": "No avatar file provided"
}
```

### ❌ Invalid Format (400 Bad Request)
```json
{
  "success": false,
  "error": "invalid image format: only jpg, jpeg, png, gif, webp allowed"
}
```

### ❌ File Too Large (400 Bad Request)
```json
{
  "success": false,
  "error": "image file too large: maximum 5MB for avatars, 10MB for properties"
}
```

### ❌ Unauthorized (401)
```json
{
  "success": false,
  "error": "Invalid or expired token"
}
```

### ❌ Cloudinary Upload Failed (500)
```json
{
  "success": false,
  "error": "failed to upload image"
}
```

---

## Common Mistakes & Solutions

### Mistake #1: Using "binary" instead of "form-data"
**Wrong:**
```
Body → binary → Select file
```

**Correct:**
```
Body → form-data → File type key named "avatar"
```

### Mistake #2: Wrong key name
**Wrong:**
```
Key: image, file, photo, picture
```

**Correct:**
```
Key: avatar (exactly, lowercase)
```

### Mistake #3: Token format
**Wrong:**
```
Authorization: Bearer eyJhbGciOiJIUzI1...
(manually typing "Bearer " in token field)
```

**Correct:**
```
Authorization Type: Bearer Token
Token: eyJhbGciOiJIUzI1...
(Postman adds "Bearer " automatically)
```

### Mistake #4: Expired token
**Symptom:** 401 Unauthorized error

**Solution:**
1. Login again to get fresh token
2. Access tokens expire after 15 minutes
3. Use the refresh endpoint if needed

---

## Testing Different Scenarios

### Test 1: Valid Upload
- **File**: profile.jpg (2MB)
- **Expected**: 200 OK with avatar_url

### Test 2: Invalid Format
- **File**: document.pdf
- **Expected**: 400 Bad Request

### Test 3: File Too Large
- **File**: huge-image.jpg (10MB)
- **Expected**: 400 Bad Request

### Test 4: No Authentication
- **Setup**: Remove Authorization header
- **Expected**: 401 Unauthorized

### Test 5: Missing File
- **Setup**: Send with empty form-data
- **Expected**: 400 Bad Request

---

## Validation Rules

| Rule | Value |
|------|-------|
| **Field Name** | `avatar` (required) |
| **Formats Allowed** | jpg, jpeg, png, gif, webp |
| **Max File Size** | 5 MB |
| **Authentication** | Bearer Token (required) |
| **Upload Service** | Cloudinary |
| **Storage Folder** | `avatars/` |

---

## After Upload

### Verify the Upload
1. Copy the `avatar_url` from response
2. Paste URL in browser to view uploaded image
3. Or call `GET /api/v1/users/me` to see avatar in profile

### Example Verification Request
```
GET http://localhost:8081/api/v1/users/me

Authorization: Bearer <your_token>

Response will include:
{
  "success": true,
  "data": {
    ...
    "avatar_url": "https://res.cloudinary.com/...",
    ...
  }
}
```

---

## Tips for Success

1. **Use Small Test Images**: Start with a small image (< 1MB) to test faster
2. **Check File Extension**: Ensure file has correct extension (.jpg, .png, etc.)
3. **Token Validity**: Tokens expire after 15 minutes, get a fresh one if needed
4. **Save Requests**: Save your Postman requests for reuse
5. **Environment Variables**: Use Postman variables for token and base URL

### Postman Environment Setup (Optional)
```
BASE_URL = http://localhost:8081
ACCESS_TOKEN = <your token here>

Then use:
{{BASE_URL}}/api/v1/users/avatar
Authorization: Bearer {{ACCESS_TOKEN}}
```

---

## Need Help?

### Check Server Logs
If upload fails, check the server console for detailed errors:
```bash
# Look for these log messages:
- "Avatar uploaded successfully"
- "Failed to upload avatar"
- "Invalid file type"
- etc.
```

### Debug Checklist
- [ ] Server is running on port 8081
- [ ] You're logged in with valid token
- [ ] Using PATCH method (not POST or PUT)
- [ ] Body type is form-data (not raw or binary)
- [ ] Key is named "avatar" and set to File type
- [ ] File is selected and visible in Value column
- [ ] File is a supported image format
- [ ] File size is under 5MB
- [ ] Authorization header is present with Bearer token

---

## Supported Image Formats

| Format | Extension | Notes |
|--------|-----------|-------|
| JPEG | .jpg, .jpeg | Most common, good compression |
| PNG | .png | Supports transparency |
| GIF | .gif | Supports animation |
| WebP | .webp | Modern format, best compression |

**Not Supported:**
- ❌ .bmp (too large)
- ❌ .tiff (not web-friendly)
- ❌ .svg (vector, not raster)
- ❌ .ico (icon format)
- ❌ .pdf (document, not image)

---

**Last Updated**: 2026-08-10  
**API Version**: v1  
**Endpoint**: PATCH /api/v1/users/avatar
