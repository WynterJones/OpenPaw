package llm

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestStripBinaryFields_StripsBase64(t *testing.T) {
	// Create a fake base64-encoded "image" (600+ chars of valid base64)
	fakeImage := base64.StdEncoding.EncodeToString(make([]byte, 1024))

	input := map[string]interface{}{
		"url":          "https://example.com",
		"width":        1280,
		"height":       720,
		"format":       "png",
		"size_bytes":   1024,
		"image_base64": fakeImage,
	}
	inputJSON, _ := json.Marshal(input)

	result := StripBinaryFields(string(inputJSON))

	var output map[string]interface{}
	if err := json.Unmarshal([]byte(result), &output); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	// Metadata should be preserved
	if output["url"] != "https://example.com" {
		t.Errorf("url was modified: %v", output["url"])
	}
	if output["format"] != "png" {
		t.Errorf("format was modified: %v", output["format"])
	}

	// Base64 field should be replaced with a short placeholder
	stripped, ok := output["image_base64"].(string)
	if !ok {
		t.Fatalf("image_base64 is not a string: %T", output["image_base64"])
	}
	if len(stripped) > 200 {
		t.Errorf("image_base64 was not stripped, length: %d", len(stripped))
	}
	if !strings.Contains(stripped, "binary data stripped") {
		t.Errorf("expected placeholder message, got: %s", stripped)
	}
}

func TestStripBinaryFields_PreservesSmallValues(t *testing.T) {
	input := map[string]interface{}{
		"label":  "test",
		"status": "ok",
		"data":   "small value",
	}
	inputJSON, _ := json.Marshal(input)

	result := StripBinaryFields(string(inputJSON))

	var output map[string]interface{}
	json.Unmarshal([]byte(result), &output)

	if output["data"] != "small value" {
		t.Errorf("small data field was incorrectly stripped: %v", output["data"])
	}
}

func TestStripBinaryFields_PreservesNonBase64(t *testing.T) {
	// Long string that is NOT base64
	longText := strings.Repeat("Hello, this is not base64! ", 50)

	input := map[string]interface{}{
		"data": longText,
	}
	inputJSON, _ := json.Marshal(input)

	result := StripBinaryFields(string(inputJSON))

	var output map[string]interface{}
	json.Unmarshal([]byte(result), &output)

	if output["data"] != longText {
		t.Error("non-base64 long text was incorrectly stripped")
	}
}

func TestStripBinaryFields_InvalidJSON(t *testing.T) {
	input := "not json at all"
	result := StripBinaryFields(input)
	if result != input {
		t.Errorf("expected input returned unchanged for non-JSON, got: %s", result)
	}
}

func TestStripBinaryFields_PDFBase64(t *testing.T) {
	fakePDF := base64.StdEncoding.EncodeToString(make([]byte, 2048))

	input := map[string]interface{}{
		"format":     "pdf",
		"size_bytes": 2048,
		"pdf_base64": fakePDF,
	}
	inputJSON, _ := json.Marshal(input)

	result := StripBinaryFields(string(inputJSON))

	var output map[string]interface{}
	json.Unmarshal([]byte(result), &output)

	stripped := output["pdf_base64"].(string)
	if !strings.Contains(stripped, "binary data stripped") {
		t.Errorf("pdf_base64 was not stripped: %s", stripped)
	}
	if output["format"] != "pdf" {
		t.Errorf("format was modified: %v", output["format"])
	}
}
