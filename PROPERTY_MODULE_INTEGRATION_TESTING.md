# Property Module Integration Testing Guide

**Date:** 2026-10-06  
**Phase:** 12 - Integration Testing  
**Status:** Ready for Execution  
**Environment:** Development (Local Docker Compose)

---

## 📋 Overview

This guide provides step-by-step instructions for manually testing the Property Module end-to-end. It covers infrastructure setup, API testing, database verification, and outbox event validation.

---

## 🎯 Testing Objectives

### **What We're Testing:**
1. ✅ Property CRUD operations
2. ✅ Image upload and management
3. ✅ Document upload and management
4. ✅ Property publishing flow
5. ✅ Public vs admin visibility
6. ✅ Soft deletion
7. ✅ Outbox event creation
8. ✅ Database constraints
9. ✅ Authentication and authorization
10. ✅ Error handling

---

## 🚀 Pre-Test Setup

### **Step 1: Start Infrastructure**

```powershell
# Start all services (PostgreSQL, RabbitMQ, Redis)
docker-compose up -d

# Verify services are running
docker-compose ps

# Expected output:
# postgres    Up    5432->5432
# rabbitmq    Up    5672->5672, 15672->15672
# redis       Up    6379->6379
```

### **Step 2: Run Migrations**

```powershell
# Set database URL
$env:DATABASE_URL = "postgresql://propvest_user:propvest_pass@localhost:5432/propvest_db?sslmode=disable"

# Run migrations (should go from 19 to 20)
migrate -path internal/database/migrations -database $env:DATABASE_URL up

# Verify migration version
migrate -path internal/database/migrations -database $env:DATABASE_URL version
# Expected: 20 (includes soft delete migration)
```

### **Step 3: Verify Database Schema**

```powershell
# Connect to PostgreSQL
docker exec -it propvest-postgres psql -U propvest_user -d propvest_db

# Inside psql:
\dt                          # List all tables
\d properties                # Describe properties table
\d property_images           # Should have deleted_at column
\d property_documents        # Should have deleted_at column
\d property_status_history
\d outbox_events

# Expected tables:
# - properties
# - property_images
# - property_documents
# - property_status_history
# - outbox_events
# - users
# - wallets
# - ... (other existing tables)

\q  # Exit psql
```

### **Step 4: Build and Start API**

```powershell
# Build the API
go build -o api.exe ./cmd/api

# Start the API (in a separate terminal)
./api.exe

# Expected output:
# [timestamp] level=INFO msg="starting PropVest API" env=development
# [timestamp] level=INFO msg="HTTP server listening" addr=:8080
```

### **Step 5: Build and Start Worker (Optional)**

```powershell
# In another terminal
go build -o worker.exe ./cmd/worker

# Start the worker
./worker.exe

# This will process outbox events
```

---

## 👤 Test User Setup

### **Register Admin User**

```powershell
# Register a new user
curl -X POST http://localhost:8080/api/v1/auth/register `
  -H "Content-Type: application/json" `
  -d '{
    "email": "admin@propvest.test",
    "password": "Admin123!@#",
    "first_name": "Admin",
    "last_name": "User",
    "phone_number": "+2348012345678"
  }'

# Expected response:
# {
#   "message": "Registration successful",
#   "data": {
#     "user": { ... },
#     "access_token": "eyJhbGc...",
#     "refresh_token": "eyJhbGc..."
#   }
# }

# IMPORTANT: Save the access_token for subsequent requests
```

### **Manually Set Admin Role**

```powershell
# Connect to database
docker exec -it propvest-postgres psql -U propvest_user -d propvest_db

# Update user role to admin
UPDATE users SET role = 'admin' WHERE email = 'admin@propvest.test';

# Verify
SELECT id, email, role FROM users WHERE email = 'admin@propvest.test';

\q
```

### **Login as Admin**

```powershell
# Login to get fresh token with admin role
curl -X POST http://localhost:8080/api/v1/auth/login `
  -H "Content-Type: application/json" `
  -d '{
    "email": "admin@propvest.test",
    "password": "Admin123!@#"
  }'

# Save the new access_token
$TOKEN = "eyJhbGc..."  # Replace with actual token
```

---

## 🏢 Property Testing Scenarios

### **Test 1: Create Draft Property**

```powershell
# Create a draft property
curl -X POST http://localhost:8080/api/v1/admin/properties `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "title": "Luxury Apartment Complex - Lekki Phase 1",
    "description": "Modern 3-bedroom apartments with state-of-the-art facilities",
    "property_type": "residential",
    "address": "Block 5, Admiralty Way",
    "city": "Lagos",
    "state": "Lagos",
    "target_amount": 50000000000,
    "unit_price": 500000000,
    "total_units": 100,
    "minimum_investment": 5000000000,
    "roi_percent": 18.5,
    "duration_months": 24,
    "launch_date": "2027-01-15T00:00:00Z"
  }'

# Expected response (201 Created):
# {
#   "message": "Property created successfully",
#   "data": {
#     "property_id": "uuid-here",
#     "title": "Luxury Apartment Complex - Lekki Phase 1",
#     "slug": "luxury-apartment-complex-lekki-phase-1",
#     "status": "draft"
#   }
# }

# IMPORTANT: Save the property_id for subsequent tests
$PROPERTY_ID = "uuid-here"
```

### **Test 2: Verify Draft Not Public**

```powershell
# Try to get property as public (no auth) - should fail
curl -X GET http://localhost:8080/api/v1/properties/$PROPERTY_ID

# Expected response (404 Not Found):
# {
#   "error": "Property not found"
# }
```

### **Test 3: Get Property as Admin**

```powershell
# Get property as admin - should succeed
curl -X GET http://localhost:8080/api/v1/admin/properties/$PROPERTY_ID `
  -H "Authorization: Bearer $TOKEN"

# Expected response (200 OK):
# {
#   "data": {
#     "id": "uuid",
#     "title": "Luxury Apartment Complex - Lekki Phase 1",
#     "status": "draft",
#     ...
#   }
# }
```

### **Test 4: Upload Property Image**

```powershell
# Create a test image file (or use existing)
# For testing, create a small test image:
# Create-Item -Path "test-image.jpg" -ItemType File

# Upload image
curl -X POST http://localhost:8080/api/v1/admin/properties/$PROPERTY_ID/images `
  -H "Authorization: Bearer $TOKEN" `
  -F "image=@test-image.jpg" `
  -F "alt_text=Exterior view of the complex" `
  -F "display_order=0" `
  -F "is_cover=true"

# Expected response (201 Created):
# {
#   "message": "Image uploaded successfully",
#   "data": {
#     "image_id": "uuid",
#     "url": "https://res.cloudinary.com/...",
#     "is_cover": true
#   }
# }

# NOTE: This requires Cloudinary credentials in .env
# If Cloudinary is not configured, this test will fail with appropriate error
```

### **Test 5: Upload Property Document**

```powershell
# Create a test PDF (or use existing)
# For testing: Create-Item -Path "test-document.pdf" -ItemType File

# Upload document
curl -X POST http://localhost:8080/api/v1/admin/properties/$PROPERTY_ID/documents `
  -H "Authorization: Bearer $TOKEN" `
  -F "document=@test-document.pdf" `
  -F "name=Property Title Deed" `
  -F "document_type=title_document" `
  -F "is_public=false"

# Expected response (201 Created):
# {
#   "message": "Document uploaded successfully",
#   "data": {
#     "document_id": "uuid",
#     "url": "https://res.cloudinary.com/...",
#     "name": "Property Title Deed",
#     "is_public": false
#   }
# }
```

### **Test 6: Try to Publish (Should Fail - No Images)**

```powershell
# Attempt to publish without meeting requirements
curl -X POST http://localhost:8080/api/v1/admin/properties/$PROPERTY_ID/publish `
  -H "Authorization: Bearer $TOKEN"

# Expected response (422 Unprocessable Entity):
# {
#   "error": "Property must have at least one image before publishing"
# }
```

### **Test 7: Publish Property**

```powershell
# After uploading images, publish the property
curl -X POST http://localhost:8080/api/v1/admin/properties/$PROPERTY_ID/publish `
  -H "Authorization: Bearer $TOKEN"

# Expected response (200 OK):
# {
#   "message": "Property published successfully",
#   "data": {
#     "property_id": "uuid",
#     "status": "active",
#     "published_at": "2026-10-06T..."
#   }
# }
```

### **Test 8: Verify Property Now Public**

```powershell
# Get property as public (no auth) - should now succeed
curl -X GET http://localhost:8080/api/v1/properties/$PROPERTY_ID

# Expected response (200 OK):
# {
#   "data": {
#     "id": "uuid",
#     "title": "Luxury Apartment Complex - Lekki Phase 1",
#     "status": "active",
#     "images": [...],
#     "documents": [...]  # Only public documents visible
#   }
# }
```

### **Test 9: List Public Properties**

```powershell
# List all active properties
curl -X GET "http://localhost:8080/api/v1/properties?page=1&page_size=10"

# Expected response (200 OK):
# {
#   "data": {
#     "properties": [
#       {
#         "id": "uuid",
#         "title": "Luxury Apartment Complex - Lekki Phase 1",
#         "status": "active",
#         ...
#       }
#     ],
#     "pagination": {
#       "page": 1,
#       "page_size": 10,
#       "total_records": 1,
#       "total_pages": 1
#     }
#   }
# }
```

### **Test 10: Filter Properties**

```powershell
# Filter by city
curl -X GET "http://localhost:8080/api/v1/properties?city=Lagos"

# Filter by property type
curl -X GET "http://localhost:8080/api/v1/properties?property_type=residential"

# Filter by ROI range
curl -X GET "http://localhost:8080/api/v1/properties?min_roi=15&max_roi=20"

# Search by title
curl -X GET "http://localhost:8080/api/v1/properties?search=Luxury"

# Combine filters
curl -X GET "http://localhost:8080/api/v1/properties?city=Lagos&property_type=residential&min_roi=15"
```

### **Test 11: Update Property**

```powershell
# Update property description
curl -X PATCH http://localhost:8080/api/v1/admin/properties/$PROPERTY_ID `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{
    "description": "Updated: Modern 3-bedroom apartments with premium amenities"
  }'

# Expected response (200 OK):
# {
#   "message": "Property updated successfully",
#   "data": { ... }
# }
```

### **Test 12: Delete Property Image**

```powershell
# First, get property to find image ID
curl -X GET http://localhost:8080/api/v1/admin/properties/$PROPERTY_ID `
  -H "Authorization: Bearer $TOKEN"

# Delete image (use image_id from response)
$IMAGE_ID = "uuid-from-above"
curl -X DELETE http://localhost:8080/api/v1/admin/properties/$PROPERTY_ID/images/$IMAGE_ID `
  -H "Authorization: Bearer $TOKEN"

# Expected response (200 OK):
# {
#   "message": "Image deleted successfully"
# }
```

### **Test 13: Soft Delete Property**

```powershell
# Soft delete the property
curl -X DELETE http://localhost:8080/api/v1/admin/properties/$PROPERTY_ID `
  -H "Authorization: Bearer $TOKEN"

# Expected response (200 OK):
# {
#   "message": "Property deleted successfully"
# }
```

### **Test 14: Verify Soft Deletion**

```powershell
# Try to get deleted property as public - should fail
curl -X GET http://localhost:8080/api/v1/properties/$PROPERTY_ID

# Expected response (404 Not Found)

# Verify in database that deleted_at is set
docker exec -it propvest-postgres psql -U propvest_user -d propvest_db

SELECT id, title, status, deleted_at FROM properties WHERE id = '$PROPERTY_ID';

# Should show deleted_at timestamp
```

---

## 🔒 Authorization Testing

### **Test 15: Access Control - No Auth**

```powershell
# Try to create property without auth token
curl -X POST http://localhost:8080/api/v1/admin/properties `
  -H "Content-Type: application/json" `
  -d '{"title": "Test"}'

# Expected response (401 Unauthorized):
# {
#   "error": "Unauthorized"
# }
```

### **Test 16: Access Control - Non-Admin User**

```powershell
# Register regular user
curl -X POST http://localhost:8080/api/v1/auth/register `
  -H "Content-Type: application/json" `
  -d '{
    "email": "user@propvest.test",
    "password": "User123!@#",
    "first_name": "Regular",
    "last_name": "User",
    "phone_number": "+2348012345679"
  }'

# Save user token
$USER_TOKEN = "user-token-here"

# Try to create property as regular user
curl -X POST http://localhost:8080/api/v1/admin/properties `
  -H "Authorization: Bearer $USER_TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"title": "Test"}'

# Expected response (403 Forbidden):
# {
#   "error": "Insufficient permissions"
# }
```

---

## 📊 Database Verification

### **Test 17: Verify Outbox Events**

```powershell
# Connect to database
docker exec -it propvest-postgres psql -U propvest_user -d propvest_db

# Check outbox events
SELECT 
    id, 
    event_type, 
    aggregate_type, 
    aggregate_id, 
    status, 
    created_at 
FROM outbox_events 
WHERE event_type = 'property.published' 
ORDER BY created_at DESC 
LIMIT 5;

# Expected: Should see property.published events
# Status should be 'pending' or 'published' depending on worker status

\q
```

### **Test 18: Verify Status History**

```powershell
# Check property status history
docker exec -it propvest-postgres psql -U propvest_user -d propvest_db

SELECT 
    property_id,
    from_status,
    to_status,
    changed_by,
    created_at
FROM property_status_history
WHERE property_id = '$PROPERTY_ID'
ORDER BY created_at;

# Expected: Should show status transitions (e.g., draft → active)

\q
```

### **Test 19: Verify Soft Delete in Database**

```powershell
docker exec -it propvest-postgres psql -U propvest_user -d propvest_db

# Check images soft delete
SELECT id, property_id, url, deleted_at 
FROM property_images 
WHERE property_id = '$PROPERTY_ID';

# Check documents soft delete
SELECT id, property_id, name, deleted_at 
FROM property_documents 
WHERE property_id = '$PROPERTY_ID';

\q
```

### **Test 20: Verify Database Constraints**

```powershell
docker exec -it propvest-postgres psql -U propvest_user -d propvest_db

# Try to insert invalid property type (should fail)
INSERT INTO properties (
    title, slug, description, property_type, status,
    target_amount, unit_price, total_units, minimum_investment,
    roi_percent, duration_months, address, city, state, country
) VALUES (
    'Test', 'test-slug', 'desc', 'invalid_type', 'draft',
    1000000, 10000, 100, 100000,
    15.5, 12, '123 St', 'Lagos', 'Lagos', 'Nigeria'
);

# Expected error: CHECK constraint "chk_property_type" violated

\q
```

---

## ✅ Test Checklist

### **Infrastructure:**
- [ ] Docker Compose services running
- [ ] Migrations applied successfully (version 20)
- [ ] API server started
- [ ] Worker started (optional)

### **User Setup:**
- [ ] Admin user registered
- [ ] Admin role assigned
- [ ] Admin login successful
- [ ] Access token obtained

### **Property CRUD:**
- [ ] Create draft property (201)
- [ ] Draft not visible publicly (404)
- [ ] Draft visible to admin (200)
- [ ] Update property (200)
- [ ] Delete property (200)
- [ ] Verify soft deletion in DB

### **Media Management:**
- [ ] Upload image (201) *requires Cloudinary*
- [ ] Set cover image (200)
- [ ] Delete image (200)
- [ ] Upload document (201) *requires Cloudinary*
- [ ] Delete document (200)
- [ ] Verify Cloudinary -deleted folders

### **Publishing Flow:**
- [ ] Publish without images (422 error)
- [ ] Publish with images (200)
- [ ] Property becomes public after publish
- [ ] Outbox event created

### **Query Operations:**
- [ ] List public properties (200)
- [ ] Filter by city (200)
- [ ] Filter by status (200)
- [ ] Filter by ROI range (200)
- [ ] Search by text (200)
- [ ] Pagination works

### **Authorization:**
- [ ] No auth rejected (401)
- [ ] Non-admin rejected (403)
- [ ] Admin access granted (200)

### **Database:**
- [ ] Outbox events created
- [ ] Status history recorded
- [ ] Soft delete works
- [ ] Constraints enforced

---

## 🐛 Common Issues & Solutions

### **Issue 1: Migration Fails**
```
Error: Dirty database version X. Fix and force version.
```

**Solution:**
```powershell
migrate -path internal/database/migrations -database $env:DATABASE_URL force X
migrate -path internal/database/migrations -database $env:DATABASE_URL up
```

### **Issue 2: API Won't Start**
```
Error: failed to connect to database
```

**Solution:**
```powershell
# Verify PostgreSQL is running
docker-compose ps

# Check DATABASE_URL in .env matches docker-compose.yml
# Default: postgresql://propvest_user:propvest_pass@localhost:5432/propvest_db?sslmode=disable
```

### **Issue 3: Cloudinary Upload Fails**
```
Error: Cloudinary not configured
```

**Solution:**
```powershell
# Add to .env:
CLOUDINARY_CLOUD_NAME=your-cloud-name
CLOUDINARY_API_KEY=your-api-key
CLOUDINARY_API_SECRET=your-api-secret

# Restart API
```

### **Issue 4: Token Expired**
```
Error: Token is expired
```

**Solution:**
```powershell
# Login again to get fresh token
curl -X POST http://localhost:8080/api/v1/auth/login ...
```

---

## 📝 Test Results Template

```markdown
## Property Module Integration Test Results

**Date:** [DATE]
**Tester:** [NAME]
**Environment:** Development

### Summary
- Total Tests: 20
- Passed: __
- Failed: __
- Skipped: __ (note reason)

### Detailed Results

#### Infrastructure Setup
- [ ] Services started: PASS/FAIL
- [ ] Migrations applied: PASS/FAIL
- [ ] API running: PASS/FAIL

#### Property CRUD
- [ ] Create: PASS/FAIL
- [ ] Read: PASS/FAIL
- [ ] Update: PASS/FAIL
- [ ] Delete: PASS/FAIL

#### Media Management
- [ ] Image upload: PASS/FAIL/SKIP
- [ ] Document upload: PASS/FAIL/SKIP

#### Publishing
- [ ] Publish flow: PASS/FAIL
- [ ] Visibility rules: PASS/FAIL

#### Authorization
- [ ] Access control: PASS/FAIL

#### Database
- [ ] Outbox events: PASS/FAIL
- [ ] Constraints: PASS/FAIL

### Issues Found
1. [Issue description]
2. [Issue description]

### Notes
[Any additional observations]
```

---

## 🎉 Success Criteria

**Phase 12 is complete when:**

✅ All infrastructure services running  
✅ Migrations applied successfully  
✅ Property CRUD operations work  
✅ Publishing flow validated  
✅ Public vs admin visibility confirmed  
✅ Soft deletion verified  
✅ Outbox events created  
✅ Database constraints enforced  
✅ Authorization working correctly  
✅ No critical bugs found  

---

## 📚 Next Steps

After completing integration testing:

1. **Document Issues:** Record any bugs found
2. **Fix Critical Issues:** Address blockers immediately
3. **Update Tests:** Add test cases for edge cases discovered
4. **Phase 13:** Complete final documentation
5. **Deploy:** Property Module ready for staging/production

---

## ✨ Congratulations!

Once all tests pass, the Property Module is **production-ready** and ready for deployment! 🚀
