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

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/storage"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/option"
)

// Global variables for command-line parameters
var (
	downloadFolder            string
	bucketName                string // This is still used for GCS client initialization if needed for processing
	projectID                 string
	impersonateServiceAccount string
	isVerbose                 bool
	pubsubTopicName           string // New: Pub/Sub topic name
	pubsubSubscriptionName    string // New: Pub/Sub subscription name
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
// Now takes bucketName and objectName as parameters directly from the Pub/Sub message
func processGCSObject(ctx context.Context, client *storage.Client, bucketName string, objectName string, downloadFolder string) error {
	downloadPath := filepath.Join(downloadFolder, objectName)

	if isVerbose {
		logf("Verbose: Attempting to process object: %s from bucket %s", objectName, bucketName)
	}

	// 1. Download the file
	rc, err := client.Bucket(bucketName).Object(objectName).NewReader(ctx)
	if err != nil {
		return fmt.Errorf("error creating reader for object %s in bucket %s: %w", objectName, bucketName, err)
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

	logf("Successfully downloaded %s from bucket %s to %s", objectName, bucketName, downloadPath)

	// 2. Delete the file from the bucket
	if err := client.Bucket(bucketName).Object(objectName).Delete(ctx); err != nil {
		logf("Warning: Error deleting object %s from bucket %s: %v", objectName, bucketName, err)
	} else {
		logf("Successfully deleted object %s from bucket %s", objectName, bucketName)
	}
	return nil
}

func main() {
	flag.StringVar(&downloadFolder, "dest", "", "Path to the folder where files will be downloaded (e.g., /app/downloads)")
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

	logf("Starting GCS file downloader (Version: %s, Built: %s)", version, buildTime)
	logf("Listening to Pub/Sub Topic: %s (Subscription: %s)", pubsubTopicName, pubsubSubscriptionName)
	logf("Destination local folder: %s", downloadFolder)
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
		if err := processGCSObject(ctx, storageClient, actualBucketName, objectName, downloadFolder); err != nil {
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
