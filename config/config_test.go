package config_test

import (
	"testing"

	"github.com/sil-org/pipedream-go/config"
)

func TestSetStructField(t *testing.T) {
	cfg := struct {
		Str string
	}{}

	err := config.SetStructField(cfg, "Str", "value for Str")
	if err == nil {
		t.Errorf("expected err from SetStructField()")
	}

	err = config.SetStructField(&cfg, "Str", "value for Str")
	if err != nil {
		t.Errorf("SetStructField() error = %v", err)
	}
	if cfg.Str != "value for Str" {
		t.Errorf("SetStructField() did not set the string field")
	}
}
