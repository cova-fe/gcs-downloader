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
This builds and pushes both the versioned tag and the `:latest` tag:
- `europe-west1-docker.pkg.dev/compute-stuff-461910/docker-images/paperless-dl:<VERSION>` (reads `<VERSION>` from `src/VERSION`)
- `europe-west1-docker.pkg.dev/compute-stuff-461910/docker-images/paperless-dl:latest`

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

--dest <path>: (Required) The path to the local folder for documents (e.g., PDFs, Word docs, OpenDocument files, text, spreadsheets, presentations).

--image-dest <path>: (Optional) The path to the local folder for downloaded images and videos. Media files will be automatically placed in year-based subdirectories (e.g., `/images/2026/photo.jpg` or `/images/2023/video.mp4`). If not specified, defaults to the `--dest` folder.

--generic-dest <path>: (Optional) The path to the local folder for generic / unclassified files (e.g., `.zip`, `.tar.gz`, `.iso`, binaries). If not specified, defaults to the `--dest` folder.

--pubsub-topic <name>: (Required) Name of the Google Cloud Pub/Sub topic to listen to.

--pubsub-subscription <name>: (Required) Name of the Google Cloud Pub/Sub subscription to use.

--project <id>: (Required) Your Google Cloud Project ID.

--bucket <name>: (Optional) Name of the GCS bucket (used if needed for client setup; actual bucket is taken from Pub/Sub notifications).

--impersonate-sa <email>: (Optional) Email of the service account to impersonate.

--verbose <bool>: (Optional) Verbose logging.

--version: (Optional) Display version and build information.

### File Classification & Routing

Downloaded files are automatically routed to one of three destination folders:

1. **Documents (`--dest`)**:
   - Files matching document types: PDF (`.pdf`), Microsoft Office (`.docx`, `.doc`, `.xlsx`, `.xls`, `.pptx`, `.ppt`), OpenDocument (`.odt`, `.odp`, `.ods`, `.odg`), Text/Markdown (`.txt`, `.md`, `.rtf`, `.csv`), eBooks (`.epub`, `.mobi`, `.djvu`), emails (`.eml`, `.msg`), etc.
   - Saved directly into `--dest/<filename>`.

2. **Images & Videos (`--image-dest`)**:
   - Photos (`.jpg`, `.png`, `.webp`, `.heic`, `.tiff`, `.gif`, `.raw`, etc.) and videos (`.mp4`, `.mov`, `.avi`, `.mkv`, `.webm`, `.m4v`, etc.).
   - Sorted into year subdirectories (`<image-dest>/<year>/<filename>`) based on EXIF metadata or filename date heuristics.
   - If no creation date is detected, placed into `<image-dest>/NO_DATE/<filename>`.
   - If a duplicate filename exists in the destination, diverted to `<image-dest>/DUPES/<filename>`.

3. **Generic Files (`--generic-dest`)**:
   - Any other unclassified files (e.g., `.zip`, `.tar.gz`, `.iso`, binaries, installer packages).
   - Saved directly into `--generic-dest/<filename>`. If `--generic-dest` is omitted, defaults to `--dest`.

### Duplicate Handling Strategies

The application uses two specialized strategies for duplicate filenames:

1. **Documents (`--dest`)**:
   - When a document with an identical filename is downloaded, a random 2-character suffix is appended to the filename (e.g., `invoice_a1.pdf`) and saved directly into the destination folder.
   - This leaves deduplication to downstream applications (such as Paperless-ngx) to process and merge automatically.

2. **Images, Videos (`--image-dest`) and Generic Files (`--generic-dest`)**:
   - When a file with an identical filename already exists in the destination folder, the application computes and compares the SHA-256 checksums of the existing file and the newly downloaded file:
     - **Identical Content (True Duplicate)**: The new file is diverted to the `DUPES/` folder (`<image-dest>/DUPES/<filename>` or `<generic-dest>/DUPES/<filename>`). If a file with that name already exists in `DUPES/`, a random 2-character suffix is appended (`photo_a1.jpg`).
     - **Different Content (Name Collision)**: The new file is saved in the **normal destination folder** with a random 2-character suffix (e.g., `<image-dest>/2026/photo_x4.jpg`), ensuring different photos or files sharing common names are not mistakenly marked as duplicates.

### Contributing
Contributions are welcome! If you find a bug or have a feature request, please open an issue or submit a pull request.

1. Fork the repository.

2. Create your feature branch (git checkout -b feature/AmazingFeature).

3. Commit your changes (git commit -m 'Add some AmazingFeature').

4. Push to the branch (git push origin feature/AmazingFeature).

5. Open a Pull Request.

## License
This project is licensed under the Apache License 2.0 - see the LICENSE file for details.
