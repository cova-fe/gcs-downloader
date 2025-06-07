package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// Global variables for command-line parameters
var (
	downloadFolder            string
	bucketName                string
	projectID                 string
	impersonateServiceAccount string
	isVerbose                 bool
	pollingInterval           time.Duration // New: For polling frequency
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

// Helper function to process a single GCS object (download and delete)
func processGCSObject(ctx context.Context, client *storage.Client, objAttrs *storage.ObjectAttrs, downloadFolder string, bucketName string) error {
	objectName := objAttrs.Name
	downloadPath := filepath.Join(downloadFolder, objectName)

	if isVerbose {
		logf("Verbose: Attempting to process object: %s", objectName)
	}

	// 1. Download the file
	rc, err := client.Bucket(bucketName).Object(objectName).NewReader(ctx)
	if err != nil {
		return fmt.Errorf("error creating reader for object %s: %w", objectName, err)
	}
	defer rc.Close()

	outFile, err := os.Create(downloadPath)
	if err != nil {
		return fmt.Errorf("error creating local file %s: %w", downloadPath, err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, rc); err != nil {
		return fmt.Errorf("error downloading object %s to %s: %w", objectName, downloadPath, err)
	}

	logf("Successfully downloaded %s to %s", objectName, downloadPath)

	// 2. Delete the file from the bucket
	if err := client.Bucket(bucketName).Object(objectName).Delete(ctx); err != nil {
		logf("Warning: Error deleting object %s from bucket: %v", objectName, err)
	} else {
		logf("Successfully deleted object %s from bucket %s", objectName, bucketName)
	}
	return nil
}

func main() {
	flag.StringVar(&downloadFolder, "dest", "", "Path to the folder where files will be downloaded (e.g., /app/downloads)")
	flag.StringVar(&bucketName, "bucket", "", "Name of the Google Cloud Storage bucket (e.g., my-unique-bucket)")
	flag.StringVar(&projectID, "project", "", "Optional: Your Google Cloud Project ID. If not provided, it will be inferred from credentials.")
	flag.StringVar(&impersonateServiceAccount, "impersonate-sa", "", "Optional: Email of the service account to impersonate (e.g., file-downloader-sa@your-project-id.iam.gserviceaccount.com)")
	flag.BoolVar(&isVerbose, "verbose", false, "Enable verbose logging.")
	flag.DurationVar(&pollingInterval, "interval", 30*time.Second, "Polling interval for checking the GCS bucket (e.g., 30s, 1m, 5m).")

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
	if bucketName == "" {
		logf("Error: --bucket parameter is required. Please specify the GCP bucket name.")
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

	logf("Starting GCS file downloader (Version: %s, Built: %s)", version, buildTime)
	logf("Source GCP bucket: %s", bucketName)
	logf("Destination local folder: %s", downloadFolder)
	if projectID != "" {
		logf("GCP Project ID: %s", projectID)
	}
	if impersonateServiceAccount != "" {
		logf("Impersonating Service Account: %s", impersonateServiceAccount)
	}
	logf("Polling interval: %s", pollingInterval)
	if isVerbose {
		logf("Verbose logging is ENABLED.")
	} else {
		logf("Verbose logging is DISABLED.")
	}

	runPollingLoop()
}

func runPollingLoop() {
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

	// Infinite loop for polling
	for {
		select {
		case <-ctx.Done():
			logf("Context cancelled. Exiting polling loop gracefully.")
			return
		case <-time.After(pollingInterval): // Wait for the specified interval
			logf("Polling bucket '%s' for new files...", bucketName)
			downloadAndCleanBucket(ctx)
		}
	}
}

func downloadAndCleanBucket(ctx context.Context) {
	var clientOptions []option.ClientOption
	if projectID != "" {
		clientOptions = append(clientOptions, option.WithQuotaProject(projectID))
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
			logf("Failed to create impersonated token source: %v", err)
			return
		}
		clientOptions = append(clientOptions, option.WithTokenSource(ts))
	}

	client, err := storage.NewClient(ctx, clientOptions...)
	if err != nil {
		logf("Error creating Google Cloud Storage client: %v", err)
		return
	}
	defer client.Close()

	it := client.Bucket(bucketName).Objects(ctx, nil)
	foundFilesThisPoll := false // Flag to track if any files were found and processed

	for {
		select {
		case <-ctx.Done():
			logf("Context cancelled during bucket iteration. Stopping download phase.")
			return
		default:
		}

		objAttrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			logf("Error iterating bucket objects: %v", err)
			return
		}

		foundFilesThisPoll = true
		if err := processGCSObject(ctx, client, objAttrs, downloadFolder, bucketName); err != nil {
			logf("Failed to process object '%s': %v", objAttrs.Name, err)
			continue
		}
	}

	if !foundFilesThisPoll {
		logf("No new files found in the GCS bucket during this poll interval.")
	} else {
		logf("Finished processing files for this poll interval.")
	}
}

// sendNotification is not used in the downloader, as it's intended for a Docker container.
// For a Docker container, notifications are typically handled by the orchestration system
// via logs (stdout/stderr) or a dedicated logging service.
func sendNotification(title, message string) {
	// This function is intentionally a no-op for this application's context.
}
