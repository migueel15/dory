package t3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Attachment is a file already uploaded to T3, ready to go with a message.
type Attachment struct {
	Type      string `json:"type"` // image or file
	ID        string `json:"id"`
	Name      string `json:"name"`
	MimeType  string `json:"mime_type"`
	SizeBytes int    `json:"size_bytes"`
}

// imageTypes are the image formats T3 accepts as images; anything else goes as a file.
var imageTypes = map[string]bool{"image/gif": true, "image/jpeg": true, "image/png": true, "image/webp": true}

// Upload stores a file in T3 as T3 Code's composer does: it mints a signed
// upload URL and posts the bytes to it.
func (c Client) Upload(ctx context.Context, name, mimeType string, data []byte) (Attachment, error) {
	if len(data) == 0 {
		return Attachment{}, fmt.Errorf("t3: empty attachment")
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	a := Attachment{Type: "file", Name: name, MimeType: mimeType, SizeBytes: len(data)}
	in := map[string]any{"type": "file", "name": name, "mimeType": mimeType, "sizeBytes": len(data)}
	if imageTypes[strings.ToLower(mimeType)] {
		a.Type, in["type"] = "image", "image"
	}
	var minted struct {
		AttachmentID string `json:"attachmentId"`
		RelativeURL  string `json:"relativeUrl"`
	}
	if err := c.call(ctx, "attachments.createUploadUrl", in, &minted); err != nil {
		return Attachment{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/"+strings.TrimLeft(minted.RelativeURL, "/"), bytes.NewReader(data))
	if err != nil {
		return Attachment{}, err
	}
	req.Header.Set("Content-Type", mimeType)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Attachment{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return Attachment{}, fmt.Errorf("t3: upload returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	a.ID = minted.AttachmentID
	return a, nil
}

// wire is the attachment as message.dispatch expects it.
func (a Attachment) wire() map[string]any {
	return map[string]any{"type": a.Type, "id": a.ID, "name": a.Name, "mimeType": a.MimeType, "sizeBytes": a.SizeBytes}
}
