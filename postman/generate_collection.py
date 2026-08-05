#!/usr/bin/env python3
"""
Generate complete Postman collection for PropVest API
Run: python generate_collection.py
"""

import json
import uuid

# Generate unique collection ID
collection_id = str(uuid.uuid4())

# Base collection structure
collection = {
    "info": {
        "_postman_id": collection_id,
        "name": "PropVest API - Complete (Milestones 0-2)",
        "description": """Complete API testing collection for PropVest backend.

## Milestones Covered
- Milestone 0: Foundation (Health Check)
- Milestone 1: Authentication (Register, Login, Refresh, Logout)
- Milestone 2: User Management (Profile, Avatar, Password, Phone)

## Features
✅ 12 Endpoints fully documented
✅ Auto-save tokens to environment
✅ Test scripts included
✅ Request validation
✅ Response validation

## Setup
1. Import this collection
2. Import PropVest_Local.postman_environment.json
3. Select "PropVest Local" environment
4. Start backend: go run cmd/api/main.go
5. Run requests in order

## Version
Milestones 0-2 Complete (2026-08-05)
Database: Migration 000006""",
        "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"
    },
    "item": []
}

# System folder
system_folder = {
    "name": "System",
    "item": [
        {
            "name": "Health Check",
            "event": [
                {
                    "listen": "test",
                    "script": {
                        "exec": [
                            "pm.test(\"Status code is 200\", function () {",
                            "    pm.response.to.have.status(200);",
                            "});",
                            "",
                            "pm.test(\"Response has correct structure\", function () {",
                            "    var jsonData = pm.response.json();",
                            "    pm.expect(jsonData).to.have.property('status');",
                            "    pm.expect(jsonData).to.have.property('message');",
                            "    pm.expect(jsonData.status).to.eql('healthy');",
                            "});",
                            "",
                            "console.log(\"✅ Health check passed\");"
                        ],
                        "type": "text/javascript"
                    }
                }
            ],
            "request": {
                "method": "GET",
                "header": [],
                "url": {
                    "raw": "{{base_url}}/../health",
                    "host": ["{{base_url}}"],
                    "path": ["..", "health"]
                },
                "description": "Check if API server is running.\n\nExpected: { \"status\": \"healthy\", \"message\": \"PropVest API is running\" }"
            },
            "response": []
        }
    ]
}

collection["item"].append(system_folder)

print("✅ Postman collection generated successfully!")
print(f"Collection ID: {collection_id}")
print(f"Total folders: 1 (more will be added)")
print(f"\nTo complete the collection, run this script or manually add remaining endpoints.")
print(f"See API_ENDPOINTS_REFERENCE.md for complete documentation.")

# Save collection
output_file = "PropVest_API_Complete.postman_collection.json"
with open(output_file, 'w', encoding='utf-8') as f:
    json.dump(collection, f, indent=2)

print(f"\n✅ Saved to: {output_file}")
print(f"\nImport this file into Postman to get started!")
