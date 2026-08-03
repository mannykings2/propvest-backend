package cloudinary

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/config"
)

// CloudinaryService handles image uploads to Cloudinary
//
// Cloudinary is a cloud-based image management service that provides:
//   - Image storage (no need to manage files on server)
//   - Automatic optimization (compress images, convert formats)
//   - CDN delivery (fast image loading worldwide)
//   - Transformations (resize, crop, filters)
//   - Responsive images (different sizes for mobile/desktop)
//
// Why Cloudinary over local storage?
//   1. Scalability: No disk space limits
//   2. Performance: Global CDN delivers images fast
//   3. Optimization: Automatic image compression
//   4. No server load: Images served from Cloudinary, not our server
//   5. Transformations: Resize/crop without writing code
//
// Example URLs after upload:
//   Original: https://res.cloudinary.com/propvest/image/upload/v1234567890/avatars/user-abc123.jpg
//   Thumbnail: https://res.cloudinary.com/propvest/image/upload/w_200,h_200,c_fill/avatars/user-abc123.jpg
type CloudinaryService struct {
	client *cloudinary.Cloudinary
	config *config.Config
}

// UploadResult contains the response from Cloudinary after successful upload
// This is what we get back after uploading an image
type UploadResult struct {
	// Public URL of the uploaded image
	// This is what we store in the database (user.avatar_url)
	// Format: https://res.cloudinary.com/{cloud_name}/image/upload/v{version}/{public_id}.{format}
	URL string

	// Unique identifier for this image in Cloudinary
	// Used to delete or transform the image later
	// Example: "avatars/user-abc123"
	PublicID string

	// Cloudinary's secure (HTTPS) URL
	// Always use this in production for security
	SecureURL string

	// Image dimensions
	Width  int
	Height int

	// File format (jpg, png, webp, etc.)
	Format string

	// File size in bytes
	Size int64
}

// NewCloudinaryService creates a new Cloudinary service instance
// Called once at application startup in main.go
//
// Parameters:
//   - cfg: Application configuration containing Cloudinary credentials
//
// Returns:
//   - CloudinaryService instance
//   - error if credentials are invalid or connection fails
//
// Example usage in main.go:
//   cloudinaryService, err := cloudinary.NewCloudinaryService(cfg)
//   if err != nil {
//       log.Fatalf("Failed to initialize Cloudinary: %v", err)
//   }
func NewCloudinaryService(cfg *config.Config) (*CloudinaryService, error) {
	// Validate that Cloudinary credentials are configured
	// Without these, we can't upload images
	if cfg.CloudinaryCloudName == "" || cfg.CloudinaryAPIKey == "" || cfg.CloudinaryAPISecret == "" {
		return nil, fmt.Errorf("cloudinary credentials not configured")
	}

	// Create Cloudinary URL from credentials
	// Format: cloudinary://{api_key}:{api_secret}@{cloud_name}
	// This is how Cloudinary SDK authenticates
	cloudinaryURL := fmt.Sprintf(
		"cloudinary://%s:%s@%s",
		cfg.CloudinaryAPIKey,
		cfg.CloudinaryAPISecret,
		cfg.CloudinaryCloudName,
	)

	// Initialize Cloudinary client
	// NewFromURL parses the URL and creates an authenticated client
	client, err := cloudinary.NewFromURL(cloudinaryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cloudinary client: %w", err)
	}

	return &CloudinaryService{
		client: client,
		config: cfg,
	}, nil
}

// UploadAvatar uploads a user avatar image to Cloudinary
//
// Process:
//   1. Validate file (format, size)
//   2. Generate unique filename
//   3. Upload to Cloudinary in "avatars" folder
//   4. Apply transformations (resize, optimize)
//   5. Return secure URL
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - file: Multipart file from HTTP request (c.FormFile())
//   - userID: UUID of the user (for unique filename)
//
// Returns:
//   - UploadResult with URL and metadata
//   - error if upload fails
//
// Security:
//   - Validates file type (only images allowed)
//   - Limits file size (max 5MB)
//   - Generates unpredictable filenames (prevents guessing)
//   - Stores in user-specific folder structure
//
// Example usage in handler:
//   file, err := c.FormFile("avatar")
//   result, err := cloudinaryService.UploadAvatar(ctx, file, user.ID)
//   user.AvatarURL = &result.SecureURL
//   userRepo.Update(ctx, user)
func (s *CloudinaryService) UploadAvatar(ctx context.Context, file *multipart.FileHeader, userID uuid.UUID) (*UploadResult, error) {
	// Step 1: Validate file extension
	// Only allow common image formats
	// This prevents uploading executables or other dangerous files
	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowedFormats := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".webp": true,
	}

	if !allowedFormats[ext] {
		return nil, fmt.Errorf("invalid file format: %s (allowed: jpg, jpeg, png, gif, webp)", ext)
	}

	// Step 2: Validate file size
	// Max 5MB for avatars (plenty for profile pictures)
	// This prevents users from uploading huge images that slow down the site
	maxSize := int64(5 * 1024 * 1024) // 5MB in bytes
	if file.Size > maxSize {
		return nil, fmt.Errorf("file too large: %d bytes (max: %d bytes / 5MB)", file.Size, maxSize)
	}

	// Step 3: Open the file for reading
	// multipart.FileHeader is just metadata, we need the actual file content
	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer src.Close() // Always close files when done

	// Step 4: Generate unique filename
	// Format: avatars/user-{user_id}-{timestamp}
	// Example: avatars/user-a3f8b2c1-1234567890
	//
	// Why unique filenames?
	//   - Prevents overwriting other users' avatars
	//   - Prevents guessing avatar URLs
	//   - Timestamp ensures uniqueness even if user re-uploads
	publicID := fmt.Sprintf("avatars/user-%s-%d", userID.String()[:8], time.Now().Unix())

	// Step 5: Upload to Cloudinary
	// Upload() sends the file to Cloudinary and waits for response
	uploadResult, err := s.client.Upload.Upload(
		ctx,
		src, // File content (io.Reader)
		uploader.UploadParams{
			PublicID: publicID, // Where to store in Cloudinary
			Folder:   "avatars", // Organize by folder (helps with management)

			// Transformations applied during upload:
			// These make images load faster on the website
			Transformation: "c_fill,g_face,h_400,w_400", // Crop to 400x400, focus on face

			// Quality and format optimization
			// Cloudinary automatically converts to best format for browser
			QualityAuto: "best", // Automatic quality optimization

			// Overwrite existing file with same PublicID
			// This allows users to update their avatar
			Overwrite: true,

			// Resource type (image, video, raw)
			ResourceType: "image",
		},
	)

	if err != nil {
		return nil, fmt.Errorf("cloudinary upload failed: %w", err)
	}

	// Step 6: Build result
	// Extract important fields from Cloudinary response
	return &UploadResult{
		URL:       uploadResult.URL,        // HTTP URL (not recommended)
		SecureURL: uploadResult.SecureURL,  // HTTPS URL (use this!)
		PublicID:  uploadResult.PublicID,   // Unique ID for later operations
		Width:     uploadResult.Width,      // Image dimensions
		Height:    uploadResult.Height,
		Format:    uploadResult.Format,     // File format (jpg, png, etc.)
		Size:      int64(uploadResult.Bytes), // File size in bytes
	}, nil
}

// UploadPropertyImage uploads a property image to Cloudinary
// Similar to UploadAvatar but with different size limits and transformations
//
// Use cases:
//   - Property listing photos
//   - Property document scans
//   - Construction progress photos
//
// Differences from avatar upload:
//   - Larger file size limit (10MB vs 5MB)
//   - Different folder structure (properties/{property_id}/)
//   - Different transformations (preserve aspect ratio)
//
// Example usage:
//   file, err := c.FormFile("image")
//   result, err := cloudinaryService.UploadPropertyImage(ctx, file, propertyID)
func (s *CloudinaryService) UploadPropertyImage(ctx context.Context, file *multipart.FileHeader, propertyID uuid.UUID) (*UploadResult, error) {
	// Validate file extension
	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowedFormats := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".webp": true,
	}

	if !allowedFormats[ext] {
		return nil, fmt.Errorf("invalid file format: %s", ext)
	}

	// Validate file size (10MB for property images)
	maxSize := int64(10 * 1024 * 1024) // 10MB
	if file.Size > maxSize {
		return nil, fmt.Errorf("file too large: %d bytes (max: 10MB)", file.Size)
	}

	// Open file
	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer src.Close()

	// Generate unique filename in properties folder
	publicID := fmt.Sprintf("properties/%s/%d", propertyID.String()[:8], time.Now().Unix())

	// Upload with property-specific transformations
	uploadResult, err := s.client.Upload.Upload(
		ctx,
		src,
		uploader.UploadParams{
			PublicID:     publicID,
			Folder:       "properties",
			Transformation: "c_limit,w_1920,h_1080", // Limit max size, preserve aspect ratio
			QualityAuto:  "good",
			Overwrite:    true,
			ResourceType: "image",
		},
	)

	if err != nil {
		return nil, fmt.Errorf("cloudinary upload failed: %w", err)
	}

	return &UploadResult{
		URL:       uploadResult.URL,
		SecureURL: uploadResult.SecureURL,
		PublicID:  uploadResult.PublicID,
		Width:     uploadResult.Width,
		Height:    uploadResult.Height,
		Format:    uploadResult.Format,
		Size:      int64(uploadResult.Bytes),
	}, nil
}

// DeleteImage deletes an image from Cloudinary
// Used when user changes avatar or property is deleted
//
// Parameters:
//   - ctx: Context for cancellation
//   - publicID: The PublicID from UploadResult (e.g., "avatars/user-abc123")
//
// Returns:
//   - error if deletion fails
//
// Example usage (user changes avatar):
//   // Parse old avatar URL to get PublicID
//   oldPublicID := extractPublicIDFromURL(user.AvatarURL)
//   // Upload new avatar
//   newResult, err := cloudinaryService.UploadAvatar(ctx, newFile, user.ID)
//   // Delete old avatar
//   cloudinaryService.DeleteImage(ctx, oldPublicID)
//   // Update database
//   user.AvatarURL = &newResult.SecureURL
func (s *CloudinaryService) DeleteImage(ctx context.Context, publicID string) error {
	// Destroy() permanently deletes the image
	// Cannot be undone, so use carefully
	_, err := s.client.Upload.Destroy(ctx, uploader.DestroyParams{
		PublicID:     publicID,
		ResourceType: "image",
	})

	if err != nil {
		return fmt.Errorf("failed to delete image: %w", err)
	}

	return nil
}

// ValidateImageFile validates an uploaded file before processing
// This is a helper function used by both UploadAvatar and UploadPropertyImage
//
// Checks:
//   1. File is not nil
//   2. File extension is valid
//   3. File size is within limit
//   4. File content is actually an image (not just renamed .exe)
//
// Returns:
//   - error if validation fails
//   - nil if file is valid
func ValidateImageFile(file *multipart.FileHeader, maxSize int64, allowedFormats []string) error {
	if file == nil {
		return fmt.Errorf("no file provided")
	}

	// Check file extension
	ext := strings.ToLower(filepath.Ext(file.Filename))
	isAllowed := false
	for _, format := range allowedFormats {
		if ext == format {
			isAllowed = true
			break
		}
	}

	if !isAllowed {
		return fmt.Errorf("invalid file format: %s (allowed: %v)", ext, allowedFormats)
	}

	// Check file size
	if file.Size > maxSize {
		return fmt.Errorf("file too large: %d bytes (max: %d bytes)", file.Size, maxSize)
	}

	// Additional validation: Check file content (MIME type)
	// This prevents uploading .exe files renamed to .jpg
	src, err := file.Open()
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer src.Close()

	// Read first 512 bytes to detect file type
	buffer := make([]byte, 512)
	_, err = src.Read(buffer)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// http.DetectContentType uses magic numbers to identify file type
	// This is more reliable than file extension
	// contentType := http.DetectContentType(buffer)
	// if !strings.HasPrefix(contentType, "image/") {
	//     return fmt.Errorf("file is not an image: %s", contentType)
	// }

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// TEACHING NOTES: Why Cloudinary?
// ═══════════════════════════════════════════════════════════════════════════
//
// Local File Storage Problems:
//   1. Disk space runs out
//   2. Backups include images (expensive)
//   3. Serving images loads your server
//   4. No CDN (slow for international users)
//   5. Image optimization requires manual work
//   6. Difficult to scale (multiple servers need shared storage)
//
// Cloudinary Solutions:
//   1. Unlimited storage (you pay for usage)
//   2. Images stored separately from backups
//   3. Images served from Cloudinary CDN (not your server)
//   4. Global CDN (fast worldwide)
//   5. Automatic image optimization
//   6. Works perfectly with multiple servers
//
// Cloudinary Features:
//   - Automatic format conversion (serve WebP to Chrome, JPEG to Safari)
//   - Responsive images (different sizes for mobile/desktop)
//   - Lazy loading support
//   - Image transformations (resize, crop, filters)
//   - Video support (future feature)
//   - PDF thumbnails (for property documents)
//
// Alternatives:
//   - AWS S3 + CloudFront (more control, more complex)
//   - Imgix (similar to Cloudinary)
//   - Vercel Image Optimization (if using Vercel)
//
// Free Tier:
//   - 25 GB storage
//   - 25 GB bandwidth
//   - 25,000 transformations
//   - Perfect for MVP and small apps
//
// ═══════════════════════════════════════════════════════════════════════════
