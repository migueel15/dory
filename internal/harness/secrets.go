package harness

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var fieldName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,31}$`)

// Validate checks the prompt before asking.
func (p SecretPrompt) Validate() error {
	if strings.TrimSpace(p.Label) == "" || strings.TrimSpace(p.Reason) == "" {
		return errors.New("request_secret needs a label and a reason")
	}
	if len(p.Fields) > 8 {
		return errors.New("request_secret takes at most 8 fields")
	}
	seen := map[string]bool{}
	for _, f := range p.Fields {
		if !fieldName.MatchString(f.Name) {
			return fmt.Errorf("invalid field name %q", f.Name)
		}
		if seen[f.Name] {
			return fmt.Errorf("repeated field name %q", f.Name)
		}
		seen[f.Name] = true
	}
	return nil
}
