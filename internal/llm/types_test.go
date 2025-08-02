package llm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileFromPath(t *testing.T) {
	// Test with existing file
	filePath := "testdata/test.txt"
	file, err := NewFileFromPath(filePath)
	
	require.NoError(t, err)
	assert.Equal(t, "test.txt", file.Name)
	assert.Equal(t, "text/plain", file.ContentType)
	assert.NotEmpty(t, file.Content)
	assert.Contains(t, string(file.Content), "This is a test file")
	assert.Empty(t, file.URL)

	// Test with non-existent file
	_, err = NewFileFromPath("nonexistent.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read file")
}

func TestNewFileFromURL(t *testing.T) {
	url := "https://example.com/document.pdf"
	file, err := NewFileFromURL(url)
	
	require.NoError(t, err)
	assert.Equal(t, "document.pdf", file.Name)
	assert.Equal(t, url, file.URL)
	assert.Nil(t, file.Content)
}

func TestFileToMessage(t *testing.T) {
	// Test text file
	textFile := &File{
		Name:        "test.txt",
		ContentType: "text/plain",
		Content:     []byte("This is test content"),
	}
	
	msg, err := textFile.ToMessage()
	require.NoError(t, err)
	assert.Contains(t, msg, "File: test.txt")
	assert.Contains(t, msg, "This is test content")

	// Test binary file
	binaryFile := &File{
		Name:        "image.png",
		ContentType: "image/png",
		Content:     []byte{0x89, 0x50, 0x4E, 0x47}, // PNG header
	}
	
	msg, err = binaryFile.ToMessage()
	require.NoError(t, err)
	assert.Contains(t, msg, "File: image.png")
	assert.Contains(t, msg, "Type: image/png")
	assert.Contains(t, msg, "Size: 4 bytes")
	assert.NotContains(t, msg, string(binaryFile.Content))

	// Test URL file
	urlFile := &File{
		Name: "document.pdf",
		URL:  "https://example.com/document.pdf",
	}
	
	msg, err = urlFile.ToMessage()
	require.NoError(t, err)
	assert.Contains(t, msg, "File URL: https://example.com/document.pdf")
	assert.Contains(t, msg, "File name: document.pdf")
}

func TestContentTypeDetection(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"test.txt", "text/plain"},
		{"document.md", "text/markdown"},
		{"data.json", "application/json"},
		{"config.xml", "application/xml"},
		{"report.csv", "text/csv"},
		{"page.html", "text/html"},
		{"styles.css", "text/css"},
		{"script.js", "application/javascript"},
		{"image.png", "image/png"},
		{"photo.jpg", "image/jpeg"},
		{"animation.gif", "image/gif"},
		{"doc.pdf", "application/pdf"},
		{"unknown.xyz", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := getContentTypeFromExtension(tt.filename)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsTextFile(t *testing.T) {
	// Test text file types
	textTypes := []string{
		"text/plain",
		"text/markdown",
		"text/csv",
		"text/html",
		"text/css",
		"application/json",
		"application/xml",
		"application/javascript",
	}

	for _, contentType := range textTypes {
		assert.True(t, isTextFile(contentType), "content type %s should be identified as text", contentType)
	}

	// Test binary file types
	binaryTypes := []string{
		"image/png",
		"image/jpeg",
		"image/gif",
		"application/pdf",
		"application/octet-stream",
		"video/mp4",
		"audio/mp3",
	}

	for _, contentType := range binaryTypes {
		assert.False(t, isTextFile(contentType), "content type %s should be identified as binary", contentType)
	}
}

func TestChatCompletionOptions(t *testing.T) {
	opts := NewChatCompletionOptions()
	
	assert.Equal(t, "", opts.SystemPrompt)
	assert.Equal(t, 0, opts.MaxTokens)
	assert.Equal(t, 0.7, opts.Temperature)
	assert.Empty(t, opts.Files)
	assert.False(t, opts.Stream)

	// Test option chaining
	file1 := File{Name: "test1.txt"}
	file2 := File{Name: "test2.txt"}
	
	opts = opts.
		WithSystemPrompt("You are a helpful assistant").
		WithMaxTokens(1000).
		WithTemperature(0.8).
		WithFiles(file1, file2).
		WithStream(true)

	assert.Equal(t, "You are a helpful assistant", opts.SystemPrompt)
	assert.Equal(t, 1000, opts.MaxTokens)
	assert.Equal(t, 0.8, opts.Temperature)
	assert.Len(t, opts.Files, 2)
	assert.True(t, opts.Stream)
}

func TestMessageMarshaling(t *testing.T) {
	msg := Message{
		Role:    "user",
		Content: "Hello, world!",
	}

	jsonData, err := msg.MarshalJSON()
	require.NoError(t, err)
	
	expected := `{"role":"user","content":"Hello, world!"}`
	assert.JSONEq(t, expected, string(jsonData))
}

func TestErrorImplementation(t *testing.T) {
	err := &Error{
		Message: "test error",
		Type:    "invalid_request",
		Code:    "400",
	}

	assert.Equal(t, "LLM API Error: test error (type: invalid_request, code: 400)", err.Error())
	assert.Implements(t, (*error)(nil), err)
}

func TestFilePaths(t *testing.T) {
	// Test absolute path
	absPath, _ := filepath.Abs("testdata/test.txt")
	file, err := NewFileFromPath(absPath)
	require.NoError(t, err)
	assert.Equal(t, "test.txt", file.Name)

	// Test relative path
	file, err = NewFileFromPath("./testdata/test.txt")
	require.NoError(t, err)
	assert.Equal(t, "test.txt", file.Name)
}

func TestLargeFileHandling(t *testing.T) {
	// Create a temporary large file
	tempFile, err := os.CreateTemp("", "large-*.txt")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())

	// Write 1MB of data
	largeContent := make([]byte, 1024*1024)
	for i := range largeContent {
		largeContent[i] = 'A' + byte(i%26)
	}
	
	_, err = tempFile.Write(largeContent)
	require.NoError(t, err)
	tempFile.Close()

	file, err := NewFileFromPath(tempFile.Name())
	require.NoError(t, err)
	assert.Equal(t, filepath.Base(tempFile.Name()), file.Name)
	assert.Equal(t, len(largeContent), len(file.Content))
	assert.Equal(t, "text/plain", file.ContentType)
}

