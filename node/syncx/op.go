package syncx

import (
	"encoding/json"
	"fmt"
)

// SchemaVersion is the operation schema this build writes.
const SchemaVersion = 1

// Operation kinds.
const (
	KindPut     = "put"     // fields of a record changed (creating it if needed)
	KindArchive = "archive" // archived_at set
	KindEvent   = "event"   // an audit event was appended
)

// Op is one replicated change. (DeviceID, Seq) is its global id; Seq is
// gapless per device. Fields is a JSON object kept verbatim so that fields this
// build does not know are forwarded untouched.
type Op struct {
	DeviceID string          `json:"device_id"`
	Seq      int64           `json:"seq"`
	HLC      string          `json:"hlc"`
	Entity   string          `json:"entity"`
	EntityID string          `json:"entity_id"`
	Kind     string          `json:"kind"`
	Fields   json.RawMessage `json:"fields"`
	SchemaV  int             `json:"schema_v"`
}

// Fields is the decoded body of an Op.
//
//	Set:  field -> new value (null clears the field), last writer wins.
//	Base: field -> clock of the value the writer replaced, to tell concurrent
//	      writes from sequential ones.
//	Adds/Rems: array fields as add-wins sets. An add is tagged with the op's
//	      stamp; a removal lists the tags of the adds it observed.
//	Event: the audit event of a KindEvent op.
type Fields struct {
	Set   map[string]any       `json:"set,omitempty"`
	Base  map[string]string    `json:"base,omitempty"`
	Adds  map[string][]any     `json:"adds,omitempty"`
	Rems  map[string][]Removal `json:"rems,omitempty"`
	Event map[string]any       `json:"event,omitempty"`
}

// Removal is one removed array element and the add tags it observed.
type Removal struct {
	Elem any      `json:"e"`
	Tags []string `json:"t"`
}

// Empty reports whether the body changes nothing.
func (f Fields) Empty() bool {
	return len(f.Set) == 0 && len(f.Adds) == 0 && len(f.Rems) == 0 && len(f.Event) == 0
}

// Decode parses the body of o.
func (o Op) Decode() (Fields, error) {
	var f Fields
	if len(o.Fields) == 0 {
		return f, nil
	}
	if err := json.Unmarshal(o.Fields, &f); err != nil {
		return f, fmt.Errorf("syncx: op %s/%d: %w", o.DeviceID, o.Seq, err)
	}
	return f, nil
}

func (o Op) validate() error {
	if o.DeviceID == "" || o.Seq < 1 || o.Entity == "" || o.EntityID == "" || o.Kind == "" {
		return fmt.Errorf("syncx: incomplete op %s/%d", o.DeviceID, o.Seq)
	}
	h, err := ParseHLC(o.HLC)
	if err != nil {
		return err
	}
	if h.Device != o.DeviceID {
		return fmt.Errorf("syncx: op %s/%d: hlc device %q differs", o.DeviceID, o.Seq, h.Device)
	}
	if len(o.Fields) > 0 && !json.Valid(o.Fields) {
		return fmt.Errorf("syncx: op %s/%d: invalid fields", o.DeviceID, o.Seq)
	}
	return nil
}

// VersionVector maps a device to the highest seq such that it holds every op
// from 1 up to it.
type VersionVector map[string]int64
