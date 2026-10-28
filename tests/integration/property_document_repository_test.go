package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/models"
	"github.com/mannykings2/propvest-backend/internal/repositories"
	"github.com/stretchr/testify/require"
)

// Regression test for the property_documents ordering drift.
//
// Migration 000018 creates property_documents with NO display_order column, but
// the document repository used to order by it (and MAX(display_order) for
// insertion order). Every property with documents therefore failed with
// SQLSTATE 42703 "column display_order does not exist", which surfaced as
// HTTP 500 on GET /api/v1/properties/:id.
//
// These queries must run against real Postgres: the column simply does not
// exist, so an in-memory/sqlite double could never catch the mismatch.
func TestPropertyDocumentRepository_QueriesMatchMigrationSchema(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup()

	ctx := context.Background()
	repo := repositories.NewPropertyDocumentRepository(testDB.DB)

	userID := seedUser(t, testDB, "doc.owner@example.com")
	propertyID := seedProperty(t, testDB, userID, "document-test-property")

	// Three public documents created back-to-back, plus one private one that
	// must never appear in the public listing.
	publicNames := []string{"Deed of Assignment", "Valuation Report", "CAC Registration"}
	for i, name := range publicNames {
		require.NoError(t, repo.Create(ctx, testDocument(propertyID, userID, i, name, true), nil))
	}
	require.NoError(t, repo.Create(ctx, testDocument(propertyID, userID, 99, "Board Minutes", false), nil))

	t.Run("FindByPropertyID returns public documents in creation order", func(t *testing.T) {
		docs, err := repo.FindByPropertyID(ctx, propertyID, true)
		require.NoError(t, err, "FindByPropertyID must not reference a column the schema lacks")
		require.Len(t, docs, len(publicNames))

		for i, doc := range docs {
			require.Equal(t, publicNames[i], doc.Name, "documents should come back oldest-first")
			require.True(t, doc.IsPublic)
		}
	})

	t.Run("FindByPropertyID admin view includes private documents", func(t *testing.T) {
		docs, err := repo.FindByPropertyID(ctx, propertyID, false)
		require.NoError(t, err)
		require.Len(t, docs, len(publicNames)+1)
	})

	t.Run("FindByType filters without display_order", func(t *testing.T) {
		docs, err := repo.FindByType(ctx, propertyID, "title_document", true)
		require.NoError(t, err)
		require.Len(t, docs, len(publicNames))
	})

	t.Run("CountPublicDocuments counts public documents only", func(t *testing.T) {
		count, err := repo.CountPublicDocuments(ctx, propertyID)
		require.NoError(t, err)
		require.EqualValues(t, len(publicNames), count)
	})
}

// testDocument builds a document row matching the 000018 schema exactly
// (name/document_type/url/public_id/mime_type/file_size/is_public/uploaded_by).
func testDocument(propertyID, uploader uuid.UUID, i int, name string, isPublic bool) *models.PropertyDocument {
	return &models.PropertyDocument{
		ID:           uuid.New(),
		PropertyID:   propertyID,
		Name:         name,
		DocumentType: "title_document",
		URL:          fmt.Sprintf("https://cdn.example.test/docs/%d.pdf", i),
		PublicID:     fmt.Sprintf("doc-%d-%s", i, uuid.NewString()),
		MIMEType:     "application/pdf",
		FileSize:     1024,
		IsPublic:     isPublic,
		UploadedBy:   uploader,
		CreatedAt:    time.Now().Add(time.Duration(i) * time.Second),
	}
}

func seedUser(t *testing.T, testDB *TestDB, email string) uuid.UUID {
	t.Helper()

	id := uuid.New()
	require.NoError(t, testDB.DB.Exec(`
		INSERT INTO users (id, user_code, email, password_hash, full_name)
		VALUES (?, ?, ?, ?, ?)`,
		id, "U"+id.String()[:8], email, "not-a-real-hash", "Test Investor",
	).Error)

	return id
}

func seedProperty(t *testing.T, testDB *TestDB, creator uuid.UUID, slug string) uuid.UUID {
	t.Helper()

	id := uuid.New()
	require.NoError(t, testDB.DB.Exec(`
		INSERT INTO properties (
			id, title, slug, description, property_type, address, city, state,
			target_amount, roi_percent, duration_months, total_units, unit_price,
			minimum_investment, status, created_by
		) VALUES (
			?, ?, ?, ?, 'residential', '12 Test Street', 'Abuja', 'FCT',
			10000000000, 12, 36, 10, 1000000000, 1000000000, 'active', ?
		)`,
		id, "Document Test Property", slug,
		"A property created so the document repository queries have something to read.",
		creator,
	).Error)

	return id
}
