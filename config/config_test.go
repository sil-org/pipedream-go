package config_test

import (
	"testing"

	"github.com/sil-org/pipedream-go/config"
)

func TestSetStructField(t *testing.T) {
	cfg := struct {
		Str string
		Int int
	}{}

	err := config.SetStructField(&cfg, "Str", "value for Str")
	if err != nil {
		t.Errorf("SetStructField() error = %v", err)
	}
	if cfg.Str != "value for Str" {
		t.Errorf("SetStructField() did not set the string field")
	}

	err = config.SetStructField(&cfg, "Int", 1)
	if err != nil {
		t.Errorf("SetStructField() error = %v", err)
	}
	if cfg.Int != 1 {
		t.Errorf("SetStructField() did not set the int field")
	}

	err = config.SetStructField(&cfg, "Str", 1)
	if err == nil {
		t.Errorf("expected an error")
	}
}
