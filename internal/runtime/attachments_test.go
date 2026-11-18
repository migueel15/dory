package runtime

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoreAttachmentsFromHarnessPrompt(t *testing.T) {
	root := t.TempDir()
	m := &Manager{dataDir: filepath.Join(root, "new")}
	file, err := m.SaveAttachment("thread", "capture (1).png", "image/png", strings.NewReader("image"))
	if err != nil {
		t.Fatal(err)
	}
	original := file
	original.Path = filepath.Join(root, "old", "attachments", "thread", filepath.Base(file.Path))
	for _, text := range []string{"Una captura", ""} {
		message := Message{Role: "user", Text: withAttachments(text, []Attachment{original})}
		m.restoreAttachments("thread", &message)
		var files []Attachment
		if err := json.Unmarshal([]byte(message.Detail), &files); err != nil {
			t.Fatal(err)
		}
		if message.Text != text || len(files) != 1 || files[0] != file {
			t.Fatalf("restored %q with files %+v", message.Text, files)
		}
	}
	prompt := withAttachments("Una captura", []Attachment{original})
	message := Message{Role: "user", Text: prompt}
	m.restoreAttachments("other-thread", &message)
	if message.Text != prompt || message.Detail != "" {
		t.Fatal("an attachment from another thread was restored")
	}
}
