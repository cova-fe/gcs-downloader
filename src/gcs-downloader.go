package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/storage"
	"github.com/rwcarlsen/goexif/exif"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/option"
)

// Global variables for command-line parameters
var (
	downloadFolder            string
	imageFolder               string
	bucketName                string // This is still used for GCS client initialization if needed for processing
	projectID                 string
	impersonateServiceAccount string
	isVerbose                 bool
	pubsubTopicName           string // New: Pub/Sub topic name
	pubsubSubscriptionName    string // New: Pub/Sub subscription name
)

// Known image extensions
var imageExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
	".tif":  true,
	".tiff": true,
	".heic": true,
	".heif": true,
	".bmp":  true,
	".raw":  true,
	".cr2":  true,
	".nef":  true,
	".arw":  true,
	".dng":  true,
	".svg":  true,
	".avif": true,
	".ico":  true,
}

var (
	// Regex matching full dates like 20230815, 2023-08-15, 2023_08_15, 2023.08.15
	dateInFilenameRegex = regexp.MustCompile(`(?:^|[^0-9])((?:19|20)\d{2})[-_.]?(0[1-9]|1[0-2])[-_.]?(0[1-9]|[12]\d|3[01])(?:[^0-9]|$)`)
	// Regex matching a 4-digit year like 1999, 2024
	yearInFilenameRegex = regexp.MustCompile(`(?:^|[^0-9])((?:19|20)\d{2})(?:[^0-9]|$)`)
)

// Versioning and Build Information (These will be set by the linker at build time)
var (
	version     = "dev"
	buildTime   = "unknown"
	bundleIdent = "com.example.gcs-downloader"
)

var customLogger *log.Logger

func init() {
	customLogger = log.New(os.Stdout, "", 0)
}

// Custom log function that adds timestamp and timezone
// This will print the time in the local timezone of the container/host where it runs.
func logf(format string, v ...interface{}) {
	timestamp := time.Now().Format("2006/01/02 15:04:05 MST") // MST will be replaced by local timezone abbr (e.g., CEST)
	customLogger.Printf("%s: %s", timestamp, fmt.Sprintf(format, v...))
}

// isPDF determines whether the file is a PDF document based on extension or MIME type
func isPDF(filename string, contentType string) bool {
	if strings.ToLower(filepath.Ext(filename)) == ".pdf" {
		return true
	}
	if strings.EqualFold(contentType, "application/pdf") {
		return true
	}
	return false
}

// isImage determines whether the file is an image based on extension or MIME type
func isImage(filename string, contentType string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	if imageExtensions[ext] {
		return true
	}
	ct := strings.ToLower(contentType)
	if strings.HasPrefix(ct, "image/") {
		return true
	}
	return false
}

// sniffedIsImage inspects the initial bytes of a file to check if it's an image
func sniffedIsImage(filePath string) bool {
	f, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return false
	}
	ct := strings.ToLower(http.DetectContentType(buf[:n]))
	return strings.HasPrefix(ct, "image/")
}

// isValidYear checks if a year string represents a plausible image creation year
func isValidYear(yearStr string) bool {
	y, err := strconv.Atoi(yearStr)
	if err != nil {
		return false
	}
	currentYear := time.Now().Year()
	return y >= 1970 && y <= currentYear+1
}

// parseYearFromString attempts to extract a 4-digit year from a formatted string (e.g., EXIF date string)
func parseYearFromString(s string) string {
	matches := yearInFilenameRegex.FindStringSubmatch(s)
	if len(matches) >= 2 && isValidYear(matches[1]) {
		return matches[1]
	}
	return ""
}

// extractYearFromEXIF attempts to read EXIF metadata and extract the creation year
func extractYearFromEXIF(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	x, err := exif.Decode(f)
	if err != nil {
		return "", err
	}

	// 1. Try standard DateTime parsing
	dt, err := x.DateTime()
	if err == nil && !dt.IsZero() && isValidYear(strconv.Itoa(dt.Year())) {
		return fmt.Sprintf("%04d", dt.Year()), nil
	}

	// 2. Try specific EXIF date tags if DateTime() failed
	tags := []exif.FieldName{
		exif.DateTimeOriginal,
		exif.DateTimeDigitized,
		exif.DateTime,
	}
	for _, tag := range tags {
		val, err := x.Get(tag)
		if err == nil && val != nil {
			strVal, err := val.StringVal()
			if err == nil {
				if y := parseYearFromString(strVal); y != "" {
					return y, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no valid EXIF date found")
}

// extractYearFromFilename extracts a 4-digit year from date patterns in the filename
func extractYearFromFilename(filename string) string {
	base := filepath.Base(filename)

	// First try full date pattern YYYY-MM-DD or YYYYMMDD
	matches := dateInFilenameRegex.FindStringSubmatch(base)
	if len(matches) >= 2 && isValidYear(matches[1]) {
		return matches[1]
	}

	// Then try standalone year pattern
	yearMatches := yearInFilenameRegex.FindAllStringSubmatch(base, -1)
	for _, m := range yearMatches {
		if len(m) >= 2 && isValidYear(m[1]) {
			return m[1]
		}
	}

	return ""
}

// extractYearFromTimestamps checks timestamps (e.g. GCS LastModified) for a plausible year
func extractYearFromTimestamps(timestamps ...time.Time) string {
	for _, ts := range timestamps {
		if !ts.IsZero() && isValidYear(strconv.Itoa(ts.Year())) {
			return fmt.Sprintf("%04d", ts.Year())
		}
	}
	return ""
}

// determineImageYear tries various strategies to identify the year of image creation
func determineImageYear(filePath string, filename string, timestamps ...time.Time) (string, string) {
	if year, err := extractYearFromEXIF(filePath); err == nil && year != "" {
		return year, "EXIF"
	}
	if year := extractYearFromFilename(filename); year != "" {
		return year, "filename"
	}
	if year := extractYearFromTimestamps(timestamps...); year != "" {
		return year, "GCS metadata"
	}
	return fmt.Sprintf("%04d", time.Now().Year()), "fallback (current year)"
}

// moveFile moves a file from src to dst, handling cross-device links seamlessly
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// Fallback for cross-device rename
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	srcFile.Close()
	return os.Remove(src)
}

const randomCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// randomString generates a cryptographically secure random alphanumeric string of length n
func randomString(length int) string {
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(randomCharset))))
		if err != nil {
			b[i] = randomCharset[time.Now().UnixNano()%int64(len(randomCharset))]
		} else {
			b[i] = randomCharset[n.Int64()]
		}
	}
	return string(b)
}

// resolveImageDestinationPath resolves the destination path for an image.
// If the file already exists in <imageFolder>/<year>/<filename>, it redirects it to <imageFolder>/DUPES/<filename>.
// If <imageFolder>/DUPES/<filename> already exists, it appends a random 2-character string to the filename.
func resolveImageDestinationPath(baseImageFolder string, year string, rawFilename string) (string, bool, error) {
	filename := filepath.Base(rawFilename)
	yearDir := filepath.Join(baseImageFolder, year)
	if err := os.MkdirAll(yearDir, 0755); err != nil {
		return "", false, fmt.Errorf("error creating year directory %s: %w", yearDir, err)
	}

	primaryPath := filepath.Join(yearDir, filename)
	if _, err := os.Stat(primaryPath); os.IsNotExist(err) {
		return primaryPath, false, nil
	}

	// File already exists in the year folder; redirect to DUPES folder
	dupesDir := filepath.Join(baseImageFolder, "DUPES")
	if err := os.MkdirAll(dupesDir, 0755); err != nil {
		return "", false, fmt.Errorf("error creating DUPES directory %s: %w", dupesDir, err)
	}

	dupePath := filepath.Join(dupesDir, filename)
	if _, err := os.Stat(dupePath); os.IsNotExist(err) {
		return dupePath, true, nil
	}

	// File already exists in DUPES folder; append a random 2-character string to the filename
	ext := filepath.Ext(filename)
	nameWithoutExt := strings.TrimSuffix(filename, ext)

	for i := 0; i < 100; i++ {
		randSuffix := randomString(2)
		candidateFilename := fmt.Sprintf("%s_%s%s", nameWithoutExt, randSuffix, ext)
		candidatePath := filepath.Join(dupesDir, candidateFilename)
		if _, err := os.Stat(candidatePath); os.IsNotExist(err) {
			return candidatePath, true, nil
		}
	}

	// Fallback with timestamp in case of unexpected collisions
	fallbackFilename := fmt.Sprintf("%s_%d%s", nameWithoutExt, time.Now().UnixNano(), ext)
	return filepath.Join(dupesDir, fallbackFilename), true, nil
}

// processDownloadedFile determines the target path for a downloaded temp file based on its type and attributes,
// moves it to the appropriate destination directory, and returns the final path and log description.
func processDownloadedFile(tempPath string, objectName string, contentType string, gcsLastModified time.Time, downloadFolder string, imageFolder string) (string, string, error) {
	isPDFFile := isPDF(objectName, contentType)
	isImageFile := !isPDFFile && isImage(objectName, contentType)

	// If not identified as PDF or image yet, check content sniffing
	if !isPDFFile && !isImageFile {
		if sniffedIsImage(tempPath) {
			isImageFile = true
		}
	}

	var finalDestPath string
	var logDetails string

	if isImageFile {
		year, source := determineImageYear(tempPath, objectName, gcsLastModified)
		targetBase := imageFolder
		if targetBase == "" {
			targetBase = downloadFolder
		}
		var isDupe bool
		var err error
		finalDestPath, isDupe, err = resolveImageDestinationPath(targetBase, year, objectName)
		if err != nil {
			return "", "", err
		}
		if isDupe {
			logDetails = fmt.Sprintf("image [DUPLICATE redirected to DUPES] (year: %s from %s)", year, source)
		} else {
			logDetails = fmt.Sprintf("image (year: %s from %s)", year, source)
		}
	} else {
		finalDestPath = filepath.Join(downloadFolder, objectName)
		if err := os.MkdirAll(filepath.Dir(finalDestPath), 0755); err != nil {
			return "", "", fmt.Errorf("error creating destination directory for %s: %w", finalDestPath, err)
		}
		if isPDFFile {
			logDetails = "PDF"
		} else {
			logDetails = "file"
		}
	}

	if err := moveFile(tempPath, finalDestPath); err != nil {
		return "", "", fmt.Errorf("error moving downloaded file to destination %s: %w", finalDestPath, err)
	}

	return finalDestPath, logDetails, nil
}

// Helper function to process a single GCS object (download and delete)
// Now takes bucketName and objectName as parameters directly from the Pub/Sub message
func processGCSObject(ctx context.Context, client *storage.Client, bucketName string, objectName string, downloadFolder string, imageFolder string) error {
	if isVerbose {
		logf("Verbose: Attempting to process object: %s from bucket %s", objectName, bucketName)
	}

	// 1. Download the file
	obj := client.Bucket(bucketName).Object(objectName)
	rc, err := obj.NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			logf("Notice: Object %s does not exist in bucket %s (already processed or removed). Skipping.", objectName, bucketName)
			return nil
		}
		return fmt.Errorf("error creating reader for object %s in bucket %s: %w", objectName, bucketName, err)
	}
	defer rc.Close()

	isPDFFile := isPDF(objectName, rc.Attrs.ContentType)
	isImageFile := !isPDFFile && isImage(objectName, rc.Attrs.ContentType)

	baseDir := downloadFolder
	if isImageFile && imageFolder != "" {
		baseDir = imageFolder
	}

	tempFile, err := os.CreateTemp(baseDir, ".tmp-download-*")
	if err != nil {
		return fmt.Errorf("error creating temp file in %s: %w", baseDir, err)
	}
	tempPath := tempFile.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			os.Remove(tempPath)
		}
	}()

	if _, err := io.Copy(tempFile, rc); err != nil {
		tempFile.Close()
		return fmt.Errorf("error downloading object %s to temp file: %w", objectName, err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("error closing temp file %s: %w", tempPath, err)
	}

	finalDestPath, logDetails, err := processDownloadedFile(tempPath, objectName, rc.Attrs.ContentType, rc.Attrs.LastModified, downloadFolder, imageFolder)
	if err != nil {
		return err
	}
	cleanupTemp = false

	logf("Successfully downloaded %s %s from bucket %s to %s", logDetails, objectName, bucketName, finalDestPath)

	// 2. Delete the file from the bucket
	if err := obj.Delete(ctx); err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			logf("Notice: Object %s in bucket %s was already deleted", objectName, bucketName)
		} else {
			logf("Warning: Error deleting object %s from bucket %s: %v", objectName, bucketName, err)
		}
	} else {
		logf("Successfully deleted object %s from bucket %s", objectName, bucketName)
	}
	return nil
}

func main() {
	flag.StringVar(&downloadFolder, "dest", "", "Path to the folder where files will be downloaded (e.g., /app/downloads)")
	flag.StringVar(&imageFolder, "image-dest", "", "Optional: Path to the folder where images will be downloaded in year-based subdirectories (e.g., /app/images). If not specified, defaults to --dest.")
	flag.StringVar(&bucketName, "bucket", "", "Optional: Name of the Google Cloud Storage bucket. This is only used for GCS client initialization if --impersonate-sa is used. Pub/Sub messages will provide the actual bucket name.")
	flag.StringVar(&projectID, "project", "", "Your Google Cloud Project ID. Required for Pub/Sub client.")
	flag.StringVar(&impersonateServiceAccount, "impersonate-sa", "", "Optional: Email of the service account to impersonate (e.g., file-downloader-sa@your-project-id.iam.gserviceaccount.com)")
	flag.BoolVar(&isVerbose, "verbose", false, "Enable verbose logging.")
	flag.StringVar(&pubsubTopicName, "pubsub-topic", "", "Name of the Google Cloud Pub/Sub topic to listen to.")
	flag.StringVar(&pubsubSubscriptionName, "pubsub-subscription", "", "Name of the Google Cloud Pub/Sub subscription to use.")

	versionFlag := flag.Bool("version", false, "Display version and build information")

	flag.Parse()

	if *versionFlag {
		fmt.Printf("Application Version: %s\n", version)
		fmt.Printf("Build Time: %s\n", buildTime)
		fmt.Printf("Bundle Identifier: %s\n", bundleIdent)
		os.Exit(0)
	}

	if downloadFolder == "" {
		logf("Error: --dest parameter is required. Please specify the destination folder for downloads.")
		os.Exit(1)
	}
	if projectID == "" {
		logf("Error: --project parameter is required. Please specify your GCP Project ID.")
		os.Exit(1)
	}
	if pubsubTopicName == "" {
		logf("Error: --pubsub-topic parameter is required. Please specify the Pub/Sub topic name.")
		os.Exit(1)
	}
	if pubsubSubscriptionName == "" {
		logf("Error: --pubsub-subscription parameter is required. Please specify the Pub/Sub subscription name.")
		os.Exit(1)
	}

	if _, err := os.Stat(downloadFolder); os.IsNotExist(err) {
		logf("Destination folder '%s' does not exist. Creating it...", downloadFolder)
		if err := os.MkdirAll(downloadFolder, 0755); err != nil {
			logf("Error creating destination folder '%s': %v", downloadFolder, err)
			os.Exit(1)
		}
	} else if err != nil {
		logf("Error checking destination folder '%s': %v", downloadFolder, err)
		os.Exit(1)
	}

	if imageFolder == "" {
		imageFolder = downloadFolder
	} else {
		if _, err := os.Stat(imageFolder); os.IsNotExist(err) {
			logf("Image destination folder '%s' does not exist. Creating it...", imageFolder)
			if err := os.MkdirAll(imageFolder, 0755); err != nil {
				logf("Error creating image destination folder '%s': %v", imageFolder, err)
				os.Exit(1)
			}
		} else if err != nil {
			logf("Error checking image destination folder '%s': %v", imageFolder, err)
			os.Exit(1)
		}
	}

	logf("Starting GCS file downloader (Version: %s, Built: %s)", version, buildTime)
	logf("Listening to Pub/Sub Topic: %s (Subscription: %s)", pubsubTopicName, pubsubSubscriptionName)
	logf("Destination local folder (PDF/documents): %s", downloadFolder)
	logf("Destination local folder (Images): %s (with year subdirectories)", imageFolder)
	logf("GCP Project ID: %s", projectID)
	if impersonateServiceAccount != "" {
		logf("Impersonating Service Account: %s", impersonateServiceAccount)
	}
	if isVerbose {
		logf("Verbose logging is ENABLED.")
	} else {
		logf("Verbose logging is DISABLED.")
	}

	runPubSubListener()
}

func runPubSubListener() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	// Listen for interrupt (Ctrl+C) and terminate signals
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logf("Received signal %v, initiating graceful shutdown...", sig)
		cancel() // Trigger context cancellation
	}()

	// Pub/Sub client options
	var pubsubClientOptions []option.ClientOption
	if projectID != "" {
		pubsubClientOptions = append(pubsubClientOptions, option.WithQuotaProject(projectID))
	}
	// For impersonation, Pub/Sub client can also use the token source
	if impersonateServiceAccount != "" {
		impersonationScopes := []string{
			"https://www.googleapis.com/auth/cloud-platform", // Broader scope for Pub/Sub if impersonating
		}
		ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
			TargetPrincipal: impersonateServiceAccount,
			Scopes:          impersonationScopes,
		})
		if err != nil {
			logf("Failed to create impersonated token source for Pub/Sub client: %v", err)
			return
		}
		pubsubClientOptions = append(pubsubClientOptions, option.WithTokenSource(ts))
	}

	// 1. Create a Pub/Sub client
	pubsubClient, err := pubsub.NewClient(ctx, projectID, pubsubClientOptions...)
	if err != nil {
		logf("Error creating Pub/Sub client: %v", err)
		return
	}
	defer pubsubClient.Close()

	// 2. Get a reference to the subscription
	sub := pubsubClient.Subscription(pubsubSubscriptionName)

	logf("Waiting for messages from subscription '%s'...", pubsubSubscriptionName)

	// 3. Receive messages
	err = sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		logf("Received message (ID: %s, Attributes: %v)", msg.ID, msg.Attributes)

		// Check for the "objectId" attribute from GCS notification
		objectName, ok := msg.Attributes["objectId"]
		if !ok {
			logf("Error: 'objectId' attribute not found in Pub/Sub message. Skipping processing of message ID: %s", msg.ID)
			msg.Ack() // Acknowledge the message to prevent redelivery
			return
		}

		// Check for the "bucketId" attribute from GCS notification
		// Note: The bucketName flag is just for initial client setup. The actual bucket will come from the message.
		actualBucketName, ok := msg.Attributes["bucketId"]
		if !ok {
			logf("Error: 'bucketId' attribute not found in Pub/Sub message for object '%s'. Skipping processing of message ID: %s", objectName, msg.ID)
			msg.Ack() // Acknowledge the message to prevent redelivery
			return
		}

		// GCS client options
		var storageClientOptions []option.ClientOption
		if projectID != "" {
			storageClientOptions = append(storageClientOptions, option.WithQuotaProject(projectID))
		}
		if impersonateServiceAccount != "" {
			impersonationScopes := []string{
				"https://www.googleapis.com/auth/devstorage.read_only",
				"https://www.googleapis.com/auth/devstorage.delete_objects",
			}
			ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
				TargetPrincipal: impersonateServiceAccount,
				Scopes:          impersonationScopes,
			})
			if err != nil {
				logf("Failed to create impersonated token source for Storage client for object '%s': %v. Not processing.", objectName, err)
				msg.Ack()
				return
			}
			storageClientOptions = append(storageClientOptions, option.WithTokenSource(ts))
		}

		// Create a storage client for each message (could optimize by reusing if no impersonation)
		storageClient, err := storage.NewClient(ctx, storageClientOptions...)
		if err != nil {
			logf("Error creating Google Cloud Storage client for object '%s': %v. Not processing.", objectName, err)
			msg.Ack() // Acknowledge the message even on client creation error
			return
		}
		defer storageClient.Close()

		// Process the GCS object (download and delete)
		if err := processGCSObject(ctx, storageClient, actualBucketName, objectName, downloadFolder, imageFolder); err != nil {
			logf("Failed to process object '%s' from bucket '%s': %v", objectName, actualBucketName, err)
			// You might want to Nack the message here instead of Ack if you want it redelivered
			// for retry, but acknowledge for now to prevent infinite loops on persistent errors.
			msg.Ack() // Acknowledge to prevent redelivery if the error is non-transient
			return
		}

		msg.Ack() // Acknowledge the message to prevent redelivery after successful processing
	})

	if err != nil && err != context.Canceled { // Don't log context cancellation as an error
		logf("Error receiving messages from Pub/Sub: %v", err)
	} else if err == context.Canceled {
		logf("Pub/Sub listener stopped due to context cancellation.")
	}
}

// sendNotification is not used in the downloader, as it's intended for a Docker container.
// For a Docker container, notifications are typically handled by the orchestration system
// via logs (stdout/stderr) or a dedicated logging service.
func sendNotification(title, message string) {
	// This function is intentionally a no-op for this application's context.
}
