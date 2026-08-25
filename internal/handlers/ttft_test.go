package handlers

import "testing"

func TestNonEmptyJSONField(t *testing.T) {
	if nonEmptyJSONField(`{"type":"text_delta","text":""}`, "text") {
		t.Fatal("empty text was treated as content")
	}
	if !nonEmptyJSONField(`{"type":"text_delta","text":"hello"}`, "text") {
		t.Fatal("non-empty text was not detected")
	}
}

func TestResponseWriterDetectsFirstNonEmptyContent(t *testing.T) {
	rw := &responseWriter{}
	rw.detectContentInSSE([]byte(`event: content_block_start
data: {"type":"content_block_start","content_block":{"type":"text","text":""}}

`))
	if rw.hasContent() {
		t.Fatal("empty content block start was treated as content")
	}

	rw.detectContentInSSE([]byte(`event: content_block_delta
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}

`))
	if !rw.hasContent() {
		t.Fatal("non-empty text delta was not detected")
	}
}
