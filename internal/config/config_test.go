package config

import "testing"

func TestValidateOrigin(t *testing.T) {
	valid := []string{"https://dory.dorlab.net", "http://localhost:8080"}
	for _, o := range valid {
		if err := validateOrigin(o); err != nil {
			t.Errorf("validateOrigin(%q) returned %v", o, err)
		}
	}
	invalid := []string{"dory.dorlab.net", "ftp://dory.dorlab.net", "https://dory.dorlab.net/", "https://dory.dorlab.net/app", "https://u:p@dory.dorlab.net"}
	for _, o := range invalid {
		if err := validateOrigin(o); err == nil {
			t.Errorf("validateOrigin(%q) accepted an invalid origin", o)
		}
	}
}
