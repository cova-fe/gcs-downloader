# GCS Downloader

A command-line utility written in Go for downloading files from a gcs bucket and drop it into a local folder. I use it on a docker image running on a synology device.

## Important note/Disclaimer
This code is mainly a way that I used to see how to interact with an LLM, using a simple problem as testbed. So pelase don't blame me too much for the code created. BTW: also this README has been created almost all by the LLM :)
To say it in another words: I run the code on mi machine and it more or less works. Can't guarantee anything else.

---

## Installation


### Build

You have some targets in makefile: 
1. build-macos-arm: As I use a macos for testing, this is useful for a local quick build
2. build-linux-amd64: To create the binary for linux, good for docker images
3. build: just pick the current architecture.

### Docker

The Dockerfile uses the makefile to build an alpine-based image

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

--dest <path>: (Required) The path to the local folder.

--bucket <name>: (Required) The name of the GCS bucket.

--project <id>: (Optional) Your Google Cloud Project ID.

--impersonate-sa <email>: (Optional) Email of the service account to impersonate.

--verbose <bool>: (Optional) Verbose logging

--interval:  Polling interval

### Contributing
Contributions are welcome! If you find a bug or have a feature request, please open an issue or submit a pull request.

1. Fork the repository.

2. Create your feature branch (git checkout -b feature/AmazingFeature).

3. Commit your changes (git commit -m 'Add some AmazingFeature').

4. Push to the branch (git push origin feature/AmazingFeature).

5. Open a Pull Request.

## License
This project is licensed under the Apache License 2.0 - see the LICENSE file for details.
