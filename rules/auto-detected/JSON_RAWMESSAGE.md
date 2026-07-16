# JSON RawMessage for Polymorphic Fields

> Auto-detected by /spartan:scan-rules — review and edit as needed.

Use `json.RawMessage` for fields that accept multiple types (string or array). Provide accessor methods that handle both formats. Delay parsing until needed.

## CORRECT

```go
// Field accepts string OR array
type MessageRequest struct {
    System   json.RawMessage `json:"system,omitempty"`
    Messages []Message       `json:"messages"`
}

// Accessor handles both formats
func (r *MessageRequest) SystemText() string {
    if len(r.System) == 0 {
        return ""
    }
    // Try string first
    var s string
    if err := json.Unmarshal(r.System, &s); err == nil {
        return s
    }
    // Try array of content blocks
    var blocks []SystemContentBlock
    if err := json.Unmarshal(r.System, &blocks); err == nil {
        var text string
        for _, b := range blocks {
            if b.Type == "text" {
                text += b.Text
            }
        }
        return text
    }
    return string(r.System)
}

// Content field also polymorphic
type Message struct {
    Role    string          `json:"role"`
    Content json.RawMessage `json:"content"`
}

func (m *Message) ContentBlocks() []ContentBlock {
    // Try string first, then array
}
```

## WRONG

```go
// Assumes only one format
type MessageRequest struct {
    System string `json:"system"` // Breaks if upstream sends array
}

// No accessor, inline parsing everywhere
func processRequest(req *MessageRequest) {
    var system string
    json.Unmarshal(req.System, &system) // Duplicated logic
}
```

## Quick Reference

| Aspect | Convention |
|--------|-----------|
| Field type | `json.RawMessage` for polymorphic fields |
| Accessor | `func (t *T) FieldName() Type` method |
| Order | Try simpler format first (string before array) |
| Fallback | Return raw string if all parsing fails |
