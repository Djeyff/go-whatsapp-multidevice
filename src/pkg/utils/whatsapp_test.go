package utils

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestDetermineMediaExtension(t *testing.T) {
	tests := []struct {
		name       string
		filename   string
		mimeType   string
		wantSuffix string
	}{
		{
			name:       "DocxFromFilename",
			filename:   "report.docx",
			mimeType:   "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			wantSuffix: ".docx",
		},
		{
			name:       "XlsxFromMime",
			filename:   "",
			mimeType:   "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			wantSuffix: ".xlsx",
		},
		{
			name:       "PptxFromMime",
			filename:   "",
			mimeType:   "application/vnd.openxmlformats-officedocument.presentationml.presentation",
			wantSuffix: ".pptx",
		},
		{
			name:       "ZipFallback",
			filename:   "",
			mimeType:   "application/zip",
			wantSuffix: ".zip",
		},
		{
			name:       "AudioOgaWithCodecsParam",
			filename:   "",
			mimeType:   "audio/ogg; codecs=opus",
			wantSuffix: ".oga",
		},
		{
			name:       "ExeFromFilename",
			filename:   "installer.exe",
			mimeType:   "application/octet-stream",
			wantSuffix: ".exe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineMediaExtension(tt.filename, tt.mimeType)
			if got != tt.wantSuffix {
				t.Fatalf("determineMediaExtension() = %q, want %q", got, tt.wantSuffix)
			}
		})
	}
}

func TestExtractMediaInfoPreservesDirectPath(t *testing.T) {
	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           proto.String("https://mmg.whatsapp.net/example"),
			DirectPath:    proto.String("/v/t62.7117-24/example.enc?ccb=11-4"),
			MediaKey:      []byte("media-key"),
			FileSHA256:    []byte("file-sha"),
			FileEncSHA256: []byte("file-enc-sha"),
			FileLength:    proto.Uint64(123),
			PTT:           proto.Bool(true),
		},
	}

	mediaType, filename, url, directPath, mediaKey, fileSHA256, fileEncSHA256, fileLength := ExtractMediaInfo(msg)

	if mediaType != "audio" {
		t.Fatalf("mediaType = %q, want audio", mediaType)
	}
	if filename == "" {
		t.Fatal("filename should be generated")
	}
	if url != "https://mmg.whatsapp.net/example" {
		t.Fatalf("url = %q, want stored WhatsApp URL", url)
	}
	if directPath != "/v/t62.7117-24/example.enc?ccb=11-4" {
		t.Fatalf("directPath = %q, want WhatsApp direct path", directPath)
	}
	if string(mediaKey) != "media-key" || string(fileSHA256) != "file-sha" || string(fileEncSHA256) != "file-enc-sha" {
		t.Fatalf("media crypto fields were not preserved")
	}
	if fileLength != 123 {
		t.Fatalf("fileLength = %d, want 123", fileLength)
	}
}
