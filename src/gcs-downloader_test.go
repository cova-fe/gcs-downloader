package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsPDF(t *testing.T) {
	tests := []struct {
		filename    string
		contentType string
		expected    bool
	}{
		{"document.pdf", "", true},
		{"DOCUMENT.PDF", "", true},
		{"folder/sub/doc.pdf", "application/octet-stream", true},
		{"file.without.ext", "application/pdf", true},
		{"file.without.ext", "APPLICATION/PDF", true},
		{"photo.jpg", "image/jpeg", false},
		{"notes.txt", "text/plain", false},
		{"archive.tar.gz", "", false},
	}

	for _, tt := range tests {
		result := isPDF(tt.filename, tt.contentType)
		if result != tt.expected {
			t.Errorf("isPDF(%q, %q) = %v; expected %v", tt.filename, tt.contentType, result, tt.expected)
		}
	}
}

func TestIsImage(t *testing.T) {
	tests := []struct {
		filename    string
		contentType string
		expected    bool
	}{
		{"photo.jpg", "", true},
		{"photo.jpeg", "", true},
		{"photo.JPEG", "", true},
		{"image.png", "", true},
		{"graphic.gif", "", true},
		{"picture.webp", "", true},
		{"photo.tif", "", true},
		{"photo.tiff", "", true},
		{"camera.heic", "", true},
		{"camera.heif", "", true},
		{"bitmap.bmp", "", true},
		{"raw.cr2", "", true},
		{"raw.nef", "", true},
		{"raw.arw", "", true},
		{"raw.dng", "", true},
		{"icon.svg", "", true},
		{"image.avif", "", true},
		{"favicon.ico", "", true},
		{"custom.file", "image/jpeg", true},
		{"custom.file", "IMAGE/PNG", true},
		{"custom.file", "image/webp", true},
		{"document.pdf", "application/pdf", false},
		{"notes.txt", "text/plain", false},
		{"script.sh", "application/x-sh", false},
	}

	for _, tt := range tests {
		result := isImage(tt.filename, tt.contentType)
		if result != tt.expected {
			t.Errorf("isImage(%q, %q) = %v; expected %v", tt.filename, tt.contentType, result, tt.expected)
		}
	}
}

func TestIsVideo(t *testing.T) {
	tests := []struct {
		filename    string
		contentType string
		expected    bool
	}{
		{"video.mp4", "", true},
		{"movie.MOV", "", true},
		{"clip.avi", "", true},
		{"recording.mkv", "", true},
		{"stream.webm", "", true},
		{"old.flv", "", true},
		{"video.wmv", "", true},
		{"clip.m4v", "", true},
		{"mobile.3gp", "", true},
		{"stream.ts", "", true},
		{"cam.mts", "", true},
		{"custom.file", "video/mp4", true},
		{"custom.file", "VIDEO/QUICKTIME", true},
		{"photo.jpg", "image/jpeg", false},
		{"document.pdf", "application/pdf", false},
		{"notes.txt", "text/plain", false},
	}

	for _, tt := range tests {
		result := isVideo(tt.filename, tt.contentType)
		if result != tt.expected {
			t.Errorf("isVideo(%q, %q) = %v; expected %v", tt.filename, tt.contentType, result, tt.expected)
		}
	}
}

func TestIsMedia(t *testing.T) {
	if !isMedia("photo.jpg", "") {
		t.Errorf("expected photo.jpg to be media")
	}
	if !isMedia("video.mp4", "") {
		t.Errorf("expected video.mp4 to be media")
	}
	if isMedia("doc.pdf", "") {
		t.Errorf("expected doc.pdf not to be media")
	}
}

func TestExtractYearFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"IMG_20230514_123456.jpg", "2023"},
		{"PXL_20240102_120000.jpg", "2024"},
		{"Screenshot_2022-05-10.png", "2022"},
		{"20211231_235959.jpeg", "2021"},
		{"2020_08_19_photo.jpg", "2020"},
		{"2018.03.15_trip.png", "2018"},
		{"vacation_2019.jpg", "2019"},
		{"2015_summer.png", "2015"},
		{"scan_1998_doc.tif", "1998"},
		{"random_file_name.jpg", ""},
		{"no_digits_here.png", ""},
		{"file_123456.jpg", ""},
		{"image_3025.jpg", ""}, // Future year > currentYear+1
		{"image_1950.jpg", ""}, // Year < 1970
	}

	for _, tt := range tests {
		result := extractYearFromFilename(tt.filename)
		if result != tt.expected {
			t.Errorf("extractYearFromFilename(%q) = %q; expected %q", tt.filename, result, tt.expected)
		}
	}
}

func TestExtractYearFromTimestamps(t *testing.T) {
	t1 := time.Date(2021, time.July, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC)
	zero := time.Time{}

	if y := extractYearFromTimestamps(t1); y != "2021" {
		t.Errorf("expected 2021, got %q", y)
	}

	if y := extractYearFromTimestamps(zero, t2); y != "2023" {
		t.Errorf("expected 2023, got %q", y)
	}

	if y := extractYearFromTimestamps(zero); y != "" {
		t.Errorf("expected empty string for zero time, got %q", y)
	}
}

func TestSniffedIsMedia(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a plain text file
	txtFile := filepath.Join(tmpDir, "sample.txt")
	if err := os.WriteFile(txtFile, []byte("Hello world, this is plain text"), 0644); err != nil {
		t.Fatalf("failed to write txt file: %v", err)
	}
	if isMed, _ := sniffedIsMedia(txtFile); isMed {
		t.Errorf("expected text file not to be detected as media")
	}

	// Create a minimal valid PNG
	pngFile := filepath.Join(tmpDir, "sample.png")
	// Minimal 1x1 PNG bytes
	pngBytes := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG magic
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR header
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
		0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
		0x42, 0x60, 0x82,
	}
	if err := os.WriteFile(pngFile, pngBytes, 0644); err != nil {
		t.Fatalf("failed to write png file: %v", err)
	}
	isMed, isVid := sniffedIsMedia(pngFile)
	if !isMed || isVid {
		t.Errorf("expected png file to be detected as image (not video)")
	}
	if !sniffedIsImage(pngFile) {
		t.Errorf("expected sniffedIsImage to return true for png")
	}
}

// createMinimalExifJPEG builds a valid minimal JPEG containing an EXIF DateTime tag
func createMinimalExifJPEG(dateStr string) []byte {
	var buf bytes.Buffer

	// SOI marker
	buf.Write([]byte{0xFF, 0xD8})

	// Build EXIF TIFF body
	var tiffBuf bytes.Buffer
	// TIFF header: "II" (little endian), version 42 (0x002A), offset to IFD0 (8)
	tiffBuf.Write([]byte{'I', 'I', 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00})

	// IFD0: 1 tag entry
	// Tag count: 1 (uint16)
	binary.Write(&tiffBuf, binary.LittleEndian, uint16(1))

	// Tag: DateTime (0x0132), Type: ASCII (2), Count: len(dateStr)+1, Offset: 8 + 2 + 12 + 4 = 26
	valOffset := uint32(8 + 2 + 12 + 4)
	valBytes := append([]byte(dateStr), 0x00)

	binary.Write(&tiffBuf, binary.LittleEndian, uint16(0x0132))        // Tag DateTime
	binary.Write(&tiffBuf, binary.LittleEndian, uint16(2))             // Type ASCII
	binary.Write(&tiffBuf, binary.LittleEndian, uint32(len(valBytes))) // Count
	binary.Write(&tiffBuf, binary.LittleEndian, valOffset)             // Value offset

	// Next IFD offset: 0
	binary.Write(&tiffBuf, binary.LittleEndian, uint32(0))

	// Value data
	tiffBuf.Write(valBytes)

	// APP1 payload: "Exif\0\0" + TIFF data
	var app1Payload bytes.Buffer
	app1Payload.Write([]byte{'E', 'x', 'i', 'f', 0x00, 0x00})
	app1Payload.Write(tiffBuf.Bytes())

	// APP1 marker and length (length includes the 2 length bytes)
	buf.Write([]byte{0xFF, 0xE1})
	app1Len := uint16(app1Payload.Len() + 2)
	binary.Write(&buf, binary.BigEndian, app1Len)
	buf.Write(app1Payload.Bytes())

	// EOI marker
	buf.Write([]byte{0xFF, 0xD9})

	return buf.Bytes()
}

func TestExtractYearFromEXIF(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Valid EXIF JPEG with date 2018:07:22 15:30:00
	jpegBytes := createMinimalExifJPEG("2018:07:22 15:30:00")
	jpegPath := filepath.Join(tmpDir, "test_exif.jpg")
	if err := os.WriteFile(jpegPath, jpegBytes, 0644); err != nil {
		t.Fatalf("failed to write exif jpeg: %v", err)
	}

	year, err := extractYearFromEXIF(jpegPath)
	if err != nil {
		t.Fatalf("extractYearFromEXIF failed: %v", err)
	}
	if year != "2018" {
		t.Errorf("expected year 2018, got %q", year)
	}

	// 2. Plain text file (no EXIF)
	txtPath := filepath.Join(tmpDir, "plain.txt")
	if err := os.WriteFile(txtPath, []byte("no exif here"), 0644); err != nil {
		t.Fatalf("failed to write plain text file: %v", err)
	}
	_, err = extractYearFromEXIF(txtPath)
	if err == nil {
		t.Errorf("expected error when decoding EXIF from plain text file, got nil")
	}
}

func TestDetermineImageYear(t *testing.T) {
	tmpDir := t.TempDir()

	// Scenario 1: EXIF date takes precedence
	jpegBytes := createMinimalExifJPEG("2017:03:10 09:00:00")
	jpegPath := filepath.Join(tmpDir, "IMG_20230514.jpg") // filename says 2023, but EXIF says 2017
	if err := os.WriteFile(jpegPath, jpegBytes, 0644); err != nil {
		t.Fatalf("failed to write jpeg: %v", err)
	}
	gcsTime := time.Date(2020, time.June, 1, 0, 0, 0, 0, time.UTC)
	year, source := determineImageYear(jpegPath, "IMG_20230514.jpg", gcsTime)
	if year != "2017" || source != "EXIF" {
		t.Errorf("expected 2017 from EXIF, got year=%q, source=%q", year, source)
	}

	// Scenario 2: No EXIF -> falls back to filename
	nonExifPath := filepath.Join(tmpDir, "IMG_20230514.jpg")
	if err := os.WriteFile(nonExifPath, []byte("dummy image content"), 0644); err != nil {
		t.Fatalf("failed to write non-exif file: %v", err)
	}
	year, source = determineImageYear(nonExifPath, "IMG_20230514.jpg", gcsTime)
	if year != "2023" || source != "filename" {
		t.Errorf("expected 2023 from filename, got year=%q, source=%q", year, source)
	}

	// Scenario 3: No EXIF, no date in filename -> falls back to GCS metadata timestamp
	randomPath := filepath.Join(tmpDir, "photo.jpg")
	if err := os.WriteFile(randomPath, []byte("dummy image content"), 0644); err != nil {
		t.Fatalf("failed to write random file: %v", err)
	}
	year, source = determineImageYear(randomPath, "photo.jpg", gcsTime)
	if year != "2020" || source != "GCS metadata" {
		t.Errorf("expected 2020 from GCS metadata, got year=%q, source=%q", year, source)
	}

	// Scenario 4: No EXIF, no date in filename, no GCS metadata -> falls back to current year
	year, source = determineImageYear(randomPath, "photo.jpg", time.Time{})
	currentYearStr := time.Now().Format("2006")
	if year != currentYearStr || source != "fallback (current year)" {
		t.Errorf("expected %s from fallback, got year=%q, source=%q", currentYearStr, year, source)
	}
}

func TestMoveFile(t *testing.T) {
	tmpDir := t.TempDir()

	srcPath := filepath.Join(tmpDir, "source.txt")
	dstPath := filepath.Join(tmpDir, "subfolder", "dest.txt")

	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		t.Fatalf("failed to create destination dir: %v", err)
	}

	content := []byte("testing move file functionality")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatalf("failed to write src file: %v", err)
	}

	if err := moveFile(srcPath, dstPath); err != nil {
		t.Fatalf("moveFile failed: %v", err)
	}

	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Errorf("source file still exists after move")
	}

	readBack, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read dest file: %v", err)
	}
	if string(readBack) != string(content) {
		t.Errorf("content mismatch: got %q, expected %q", string(readBack), string(content))
	}
}

func TestProcessDownloadedFile(t *testing.T) {
	tmpBase := t.TempDir()
	downloadDir := filepath.Join(tmpBase, "downloads")
	imageDir := filepath.Join(tmpBase, "images")

	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		t.Fatalf("failed to create downloadDir: %v", err)
	}
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		t.Fatalf("failed to create imageDir: %v", err)
	}

	// 1. PDF File -> goes to downloadDir directly
	pdfTemp := filepath.Join(tmpBase, "temp_doc.tmp")
	if err := os.WriteFile(pdfTemp, []byte("%PDF-1.4 test content"), 0644); err != nil {
		t.Fatalf("failed to write temp PDF: %v", err)
	}
	destPath, logDetails, err := processDownloadedFile(pdfTemp, "invoice.pdf", "application/pdf", time.Time{}, downloadDir, imageDir)
	if err != nil {
		t.Fatalf("processDownloadedFile failed for PDF: %v", err)
	}
	expectedPDFPath := filepath.Join(downloadDir, "invoice.pdf")
	if destPath != expectedPDFPath {
		t.Errorf("PDF destPath = %q; expected %q", destPath, expectedPDFPath)
	}
	if _, err := os.Stat(expectedPDFPath); os.IsNotExist(err) {
		t.Errorf("PDF not found at expected location %q", expectedPDFPath)
	}
	if logDetails != "PDF" {
		t.Errorf("expected logDetails 'PDF', got %q", logDetails)
	}

	// 2. Image with EXIF -> goes to imageDir/<year>/<filename>
	imgTemp := filepath.Join(tmpBase, "temp_img.tmp")
	jpegBytes := createMinimalExifJPEG("2019:11:20 14:00:00")
	if err := os.WriteFile(imgTemp, jpegBytes, 0644); err != nil {
		t.Fatalf("failed to write temp JPEG: %v", err)
	}
	destPath, logDetails, err = processDownloadedFile(imgTemp, "holiday.jpg", "image/jpeg", time.Time{}, downloadDir, imageDir)
	if err != nil {
		t.Fatalf("processDownloadedFile failed for Image with EXIF: %v", err)
	}
	expectedImgPath := filepath.Join(imageDir, "2019", "holiday.jpg")
	if destPath != expectedImgPath {
		t.Errorf("Image destPath = %q; expected %q", destPath, expectedImgPath)
	}
	if _, err := os.Stat(expectedImgPath); os.IsNotExist(err) {
		t.Errorf("Image not found at expected location %q", expectedImgPath)
	}

	// 3. Image with filename date and empty imageDir -> defaults to downloadDir/<year>/<filename>
	img2Temp := filepath.Join(tmpBase, "temp_img2.tmp")
	if err := os.WriteFile(img2Temp, []byte("fake image data"), 0644); err != nil {
		t.Fatalf("failed to write temp img2: %v", err)
	}
	destPath, _, err = processDownloadedFile(img2Temp, "IMG_20220815_123456.png", "image/png", time.Time{}, downloadDir, "")
	if err != nil {
		t.Fatalf("processDownloadedFile failed for Image without dedicated imageDir: %v", err)
	}
	expectedImg2Path := filepath.Join(downloadDir, "2022", "IMG_20220815_123456.png")
	if destPath != expectedImg2Path {
		t.Errorf("Image destPath = %q; expected %q", destPath, expectedImg2Path)
	}
	if _, err := os.Stat(expectedImg2Path); os.IsNotExist(err) {
		t.Errorf("Image not found at expected location %q", expectedImg2Path)
	}

	// 4. Video file -> goes to imageDir/<year>/<filename>
	vidTemp := filepath.Join(tmpBase, "temp_vid.tmp")
	if err := os.WriteFile(vidTemp, []byte("fake video stream data"), 0644); err != nil {
		t.Fatalf("failed to write temp video: %v", err)
	}
	destPath, logDetails, err = processDownloadedFile(vidTemp, "VID_20230704_120000.mp4", "video/mp4", time.Time{}, downloadDir, imageDir)
	if err != nil {
		t.Fatalf("processDownloadedFile failed for Video: %v", err)
	}
	expectedVidPath := filepath.Join(imageDir, "2023", "VID_20230704_120000.mp4")
	if destPath != expectedVidPath {
		t.Errorf("Video destPath = %q; expected %q", destPath, expectedVidPath)
	}
	if _, err := os.Stat(expectedVidPath); os.IsNotExist(err) {
		t.Errorf("Video not found at expected location %q", expectedVidPath)
	}
	if !strings.HasPrefix(logDetails, "video") {
		t.Errorf("expected logDetails starting with 'video', got %q", logDetails)
	}

	// 5. Non-image, non-video, non-PDF file -> goes to downloadDir directly
	txtTemp := filepath.Join(tmpBase, "temp_txt.tmp")
	if err := os.WriteFile(txtTemp, []byte("just a text file"), 0644); err != nil {
		t.Fatalf("failed to write temp txt: %v", err)
	}
	destPath, logDetails, err = processDownloadedFile(txtTemp, "notes.txt", "text/plain", time.Time{}, downloadDir, imageDir)
	if err != nil {
		t.Fatalf("processDownloadedFile failed for text file: %v", err)
	}
	expectedTxtPath := filepath.Join(downloadDir, "notes.txt")
	if destPath != expectedTxtPath {
		t.Errorf("text file destPath = %q; expected %q", destPath, expectedTxtPath)
	}
	if _, err := os.Stat(expectedTxtPath); os.IsNotExist(err) {
		t.Errorf("Text file not found at expected location %q", expectedTxtPath)
	}
	if logDetails != "file" {
		t.Errorf("expected logDetails 'file', got %q", logDetails)
	}
}

func TestResolveImageDestinationPath(t *testing.T) {
	tmpDir := t.TempDir()
	year := "2026"

	// 1. First file: should go to <tmpDir>/2026/photo.jpg
	p1, isDupe, err := resolveImageDestinationPath(tmpDir, year, "photo.jpg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isDupe {
		t.Errorf("expected isDupe=false for first file")
	}
	expectedP1 := filepath.Join(tmpDir, "2026", "photo.jpg")
	if p1 != expectedP1 {
		t.Errorf("expected %q, got %q", expectedP1, p1)
	}

	// Create the file at p1 so it exists
	if err := os.WriteFile(p1, []byte("image 1"), 0644); err != nil {
		t.Fatalf("failed to write p1: %v", err)
	}

	// 2. Second file with same name: should go to <tmpDir>/DUPES/photo.jpg
	p2, isDupe, err := resolveImageDestinationPath(tmpDir, year, "photo.jpg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isDupe {
		t.Errorf("expected isDupe=true for second duplicate file")
	}
	expectedP2 := filepath.Join(tmpDir, "DUPES", "photo.jpg")
	if p2 != expectedP2 {
		t.Errorf("expected %q, got %q", expectedP2, p2)
	}

	// Create the file at p2 so it exists in DUPES
	if err := os.WriteFile(p2, []byte("image 2 (dupe 1)"), 0644); err != nil {
		t.Fatalf("failed to write p2: %v", err)
	}

	// 3. Third file with same name: should go to <tmpDir>/DUPES/photo_<random2chars>.jpg
	p3, isDupe, err := resolveImageDestinationPath(tmpDir, year, "photo.jpg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isDupe {
		t.Errorf("expected isDupe=true for third duplicate file")
	}
	if filepath.Dir(p3) != filepath.Join(tmpDir, "DUPES") {
		t.Errorf("expected dir to be DUPES, got %q", filepath.Dir(p3))
	}
	if p3 == p2 {
		t.Errorf("expected p3 to differ from p2, got %q", p3)
	}
	// Filename should match photo_??.jpg
	baseName := filepath.Base(p3)
	if len(baseName) != len("photo_XX.jpg") || filepath.Ext(baseName) != ".jpg" {
		t.Errorf("expected format photo_XX.jpg, got %q", baseName)
	}

	// Create the file at p3 so it exists
	if err := os.WriteFile(p3, []byte("image 3 (dupe 2)"), 0644); err != nil {
		t.Fatalf("failed to write p3: %v", err)
	}

	// 4. Fourth file with same name: should also get a unique random name in DUPES
	p4, isDupe, err := resolveImageDestinationPath(tmpDir, year, "photo.jpg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isDupe {
		t.Errorf("expected isDupe=true for fourth duplicate file")
	}
	if p4 == p3 || p4 == p2 {
		t.Errorf("expected p4 to be distinct from p2 and p3, got %q", p4)
	}
}

func TestRandomString(t *testing.T) {
	s1 := randomString(2)
	s2 := randomString(2)
	if len(s1) != 2 || len(s2) != 2 {
		t.Errorf("expected string length 2, got %d and %d", len(s1), len(s2))
	}
}
