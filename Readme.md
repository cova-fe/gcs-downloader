# GCS Downloader

A command-line utility written in Go for downloading files from a gcs bucket and drop it into a local folder. I use it on a docker image running on a synology device.

## Important note/Disclaimer
This code is mainly a way that I used to see how to interact with an LLM, using a simple problem as testbed. So pelase don't blame me too much for the code created. BTW: also this README has been created almost all by the LLM :)
To say it in another words: I run the code on mi machine and it more or less works. Can't guarantee anything else.

---

## Installation


### Build & Test

This project uses [Task](https://taskfile.dev/) (`task`) for build automation:
1. `task build`: Build the binary for the current architecture.
2. `task test`: Run unit tests.
3. `task secret-scan`: Scan the repository for leaked secrets using Trivy.
4. `task check`: Run formatting checks (`gofmt`), secret leak scanning (`trivy`), static analysis (`go vet`), and unit tests.
5. `task fmt`: Auto-format all Go source files.
6. `task build-macos-arm`: Build for macOS ARM (darwin/arm64).
7. `task build-linux-amd64`: Build for Linux AMD64 (linux/amd64).
8. `task cloud-build`: Build and push the container image to GCP Artifact Registry via Google Cloud Build.
9. `task install-hooks`: Install the Git pre-commit hook.
10. `task clean`: Remove local build artifacts.

### GCP Cloud Build

To build and push the container image directly to Google Cloud Artifact Registry using Google Cloud Build:
```bash
task cloud-build
```
Default target image tag:
`europe-west1-docker.pkg.dev/compute-stuff-461910/docker-images/paperless-dl:<VERSION>` (reads `<VERSION>` from `src/VERSION`).

You can also override variables on the command line:
```bash
task cloud-build VERSION=0.0.9
task cloud-build GCP_PROJECT=my-project GCP_REGION=europe-west1 IMAGE_NAME=my-downloader
```

### Pre-commit Hook & Secret Leak Prevention

To ensure tests, formatting, and secret leak scanning run automatically before committing:
```bash
task install-hooks
```
Or if you use the `pre-commit` tool:
```bash
pre-commit install
pre-commit run -a
```
The pre-commit configuration checks:
- Leaked credentials, API keys, and secrets (via `trivy` and `detect-private-key`)
- Go code formatting (`gofmt`)
- Go static analysis (`go vet`)
- Unit tests (`go test`)
- File formatting (trailing whitespace, YAML syntax, end-of-file fixers)

### Docker

The multi-stage `Dockerfile` (located in `src/Dockerfile`) compiles the Linux AMD64 binary and packages it into a minimal Alpine container.

---

## Usage

### Authentication

This tool relies on the Google Cloud client library for Go, which supports various authentication methods. The most common methods are:

1.  **Service Account Key File:**
    Set the `GOOGLE_APPLICATION_CREDENTIALS` environment variable to the path of your service account JSON key file:
    ```bash
    export GOOGLE_APPLICATION_CREDENTIALS="/path/to/your/service-account-key.json"
    ```
2.  **Application Default Credentials (ADC):**
    If you're running this on a GCP environment (e.g., GCE, Cloud Run, Cloud Functions), it will automatically use the service account associated with that environment. Locally on macOS, you can authenticate using the `gcloud` CLI:
    ```bash
    gcloud auth application-default login
    ```

#### Available Flags:

--dest <path>: (Required) The path to the local folder for downloaded files (e.g., PDFs and general documents).

--image-dest <path>: (Optional) The path to the local folder for downloaded images. Images will be automatically placed in year-based subdirectories (e.g., `/images/2026/photo.jpg`). If not specified, defaults to the `--dest` folder.

--pubsub-topic <name>: (Required) Name of the Google Cloud Pub/Sub topic to listen to.

--pubsub-subscription <name>: (Required) Name of the Google Cloud Pub/Sub subscription to use.

--project <id>: (Required) Your Google Cloud Project ID.

--bucket <name>: (Optional) Name of the GCS bucket (used if needed for client setup; actual bucket is taken from Pub/Sub notifications).

--impersonate-sa <email>: (Optional) Email of the service account to impersonate.

--verbose <bool>: (Optional) Verbose logging.

--version: (Optional) Display version and build information.

### Image Sorting by Year

When an image is downloaded (e.g. `.jpg`, `.png`, `.webp`, `.heic`, `.tiff`, etc.), the application determines the image's creation year using the following priority:
1. **EXIF Metadata:** Reads standard EXIF tags (`DateTimeOriginal`, `DateTimeDigitized`, `DateTime`).
2. **Filename Date Heuristics:** Matches timestamps and dates in filenames (e.g., `IMG_20230514_...jpg`, `Screenshot_2022-05-10.png`).
3. **GCS Metadata:** Uses the object's `LastModified` timestamp from Google Cloud Storage.
4. **Fallback:** Defaults to the current year.

The image is then placed into `<image-dest>/<year>/<filename>`. PDF and non-image documents continue to be downloaded directly into `--dest` without any changes.

### Duplicate Image Handling

If an image arrives with a filename that already exists in the destination `<image-dest>/<year>/` folder:
1. The duplicate image is automatically diverted to a `DUPES` folder at the root of the image directory (`<image-dest>/DUPES/<filename>`).
2. If a file with that name is already present in the `DUPES` folder, a random 2-character suffix is appended to the filename before the extension (e.g. `photo_a1.jpg`) to prevent overwriting.

### Contributing
Contributions are welcome! If you find a bug or have a feature request, please open an issue or submit a pull request.

1. Fork the repository.

2. Create your feature branch (git checkout -b feature/AmazingFeature).

3. Commit your changes (git commit -m 'Add some AmazingFeature').

4. Push to the branch (git push origin feature/AmazingFeature).

5. Open a Pull Request.

## License
This project is licensed under the Apache License 2.0 - see the LICENSE file for details.
