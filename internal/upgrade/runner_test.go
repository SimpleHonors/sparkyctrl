package upgrade

import (
	"fmt"
	"testing"
)

type fakeRunner struct {
	out  map[string]string // key: "name arg0 arg1..."
	err  map[string]error
	last []string
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	key := name
	for _, a := range args {
		key += " " + a
	}
	f.last = append([]string{name}, args...)
	return f.out[key], f.err[key]
}

func TestCheckVersionOK(t *testing.T) {
	r := &fakeRunner{out: map[string]string{"/tmp/new version": "0.1.14\n"}}
	if err := CheckVersion(r, "/tmp/new", "0.1.14"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckVersionMismatch(t *testing.T) {
	r := &fakeRunner{out: map[string]string{"/tmp/new version": "0.1.13\n"}}
	if err := CheckVersion(r, "/tmp/new", "0.1.14"); err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestCheckVersionRunError(t *testing.T) {
	r := &fakeRunner{err: map[string]error{"/tmp/new version": fmt.Errorf("exec fail")}}
	if err := CheckVersion(r, "/tmp/new", "0.1.14"); err == nil {
		t.Fatal("expected run error")
	}
}
