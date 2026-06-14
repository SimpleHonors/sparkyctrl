package protocol

import "testing"

func TestValidateTimeoutSec(t *testing.T) {
	cases := []struct {
		name    string
		sec     int
		wantErr bool
	}{
		{"zero means server default", 0, false},
		{"positive small", 5, false},
		{"at max", MaxTimeoutSec, false},
		{"negative rejected", -1, true},
		{"over max rejected", MaxTimeoutSec + 1, true},
		{"huge rejected", 1 << 40, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateTimeoutSec(c.sec)
			if (err != nil) != c.wantErr {
				t.Fatalf("ValidateTimeoutSec(%d) err=%v, wantErr=%v", c.sec, err, c.wantErr)
			}
		})
	}
}

func TestValidateEnv(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		wantErr bool
	}{
		{"nil ok", nil, false},
		{"normal ok", map[string]string{"FOO": "bar"}, false},
		{"empty key rejected", map[string]string{"": "bar"}, true},
		{"equals in key rejected", map[string]string{"A=B": "x"}, true},
		{"newline in key rejected", map[string]string{"FOO\nBAR": "x"}, true},
		{"nul in key rejected", map[string]string{"FOO\x00BAR": "x"}, true},
		{"value with equals ok", map[string]string{"FOO": "a=b"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateEnv(c.env)
			if (err != nil) != c.wantErr {
				t.Fatalf("ValidateEnv(%v) err=%v, wantErr=%v", c.env, err, c.wantErr)
			}
		})
	}
}
