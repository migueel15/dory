package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const attachmentFooter = "Ficheros adjuntos (léelos desde su ruta):"

var attachmentIDPrefix = regexp.MustCompile(`^run-[a-z0-9]{12}-[0-9]{1,6}-`)

// Attachment is a file the user sent with a message. It is saved on the node
// of the thread and the agent reads it from its path.
type Attachment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime"`
	Path string `json:"path"`
}

// maxAttachment bounds one uploaded file.
const maxAttachment = 25 << 20

// SaveAttachment stores an uploaded file of a thread.
func (m *Manager) SaveAttachment(threadID, name, mime string, r io.Reader) (Attachment, error) {
	dir := filepath.Join(m.dataDir, "attachments", threadID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Attachment{}, err
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		name = "fichero"
	}
	a := Attachment{ID: newRunID(), Name: name, Mime: mime}
	a.Path = filepath.Join(dir, a.ID+"-"+name)
	f, err := os.OpenFile(a.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return a, err
	}
	n, err := io.Copy(f, io.LimitReader(r, maxAttachment+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxAttachment {
		err = fmt.Errorf("el fichero pasa de %d MB", maxAttachment>>20)
	}
	if err != nil {
		os.Remove(a.Path)
	}
	return a, err
}

// AttachmentPath finds a stored file of a thread by its id.
func (m *Manager) AttachmentPath(threadID, id string) (string, error) {
	if strings.ContainsAny(id, `/\.`) || id == "" {
		return "", ErrNotFound
	}
	files, _ := filepath.Glob(filepath.Join(m.dataDir, "attachments", threadID, id+"-*"))
	if len(files) == 0 {
		return "", ErrNotFound
	}
	return files[0], nil
}

// Attachment returns a stored file of a thread with its metadata.
func (m *Manager) Attachment(threadID, id string) (Attachment, error) {
	path, err := m.AttachmentPath(threadID, id)
	if err != nil {
		return Attachment{}, err
	}
	name := strings.TrimPrefix(filepath.Base(path), id+"-")
	return Attachment{ID: id, Name: name, Mime: mime.TypeByExtension(filepath.Ext(name)), Path: path}, nil
}

// withAttachments tells the agent where the files of a message are.
func withAttachments(text string, files []Attachment) string {
	if len(files) == 0 {
		return text
	}
	var b strings.Builder
	b.WriteString(text)
	b.WriteString("\n\n" + attachmentFooter)
	for _, f := range files {
		fmt.Fprintf(&b, "\n- %s (%s)", f.Path, f.Name)
	}
	return b.String()
}

// restoreAttachments recovers Dory's attachment metadata from a harness prompt.
func (m *Manager) restoreAttachments(thread string, msg *Message) {
	if msg.Role != "user" || msg.Detail != "" {
		return
	}
	marker := "\n\n" + attachmentFooter + "\n"
	start := strings.LastIndex(msg.Text, marker)
	var text, footer string
	if start >= 0 {
		text, footer = msg.Text[:start], msg.Text[start+len(marker):]
	} else if rest, ok := strings.CutPrefix(msg.Text, attachmentFooter+"\n"); ok {
		footer = rest
	} else {
		return
	}

	var files []Attachment
	for _, line := range strings.Split(footer, "\n") {
		entry, ok := strings.CutPrefix(line, "- ")
		if !ok {
			return
		}
		id := strings.TrimSuffix(attachmentIDPrefix.FindString(filepath.Base(entry)), "-")
		if id == "" {
			return
		}
		file, err := m.Attachment(thread, id)
		if err != nil {
			return
		}
		path, ok := strings.CutSuffix(entry, " ("+file.Name+")")
		if !ok || filepath.Base(path) != id+"-"+file.Name ||
			filepath.Base(filepath.Dir(path)) != thread || filepath.Base(filepath.Dir(filepath.Dir(path))) != "attachments" {
			return
		}
		files = append(files, file)
	}
	msg.Text = text
	msg.Detail = attachmentsDetail(files)
}

// attachmentsDetail keeps the files of a user message for the interface.
func attachmentsDetail(files []Attachment) string {
	if len(files) == 0 {
		return ""
	}
	raw, _ := json.Marshal(files)
	return string(raw)
}

// Files lists up to limit files of a folder whose path contains q, from git
// when it is a repository.
func Files(ctx context.Context, dir, q string, limit int) []string {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "ls-files", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		return []string{}
	}
	q = strings.ToLower(q)
	list := []string{}
	for _, f := range strings.Split(string(out), "\n") {
		if f != "" && strings.Contains(strings.ToLower(f), q) {
			list = append(list, f)
			if len(list) == limit {
				break
			}
		}
	}
	return list
}
