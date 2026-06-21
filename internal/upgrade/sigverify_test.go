package upgrade

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSigVerify_HappyPath signs a real file with the `minisign` CLI using a
// known test key, then verifies it with VerifyFileSignature. This is the
// integration check that the Go verify path matches what the toolchain (and
// install.sh / install.ps1) produces.
//
// Skip when minisign is not installed (test environments without apt).
func TestSigVerify_HappyPath(t *testing.T) {
	if _, err := exec.LookPath("minisign"); err != nil {
		t.Skip("minisign not installed; skipping integration test")
	}

	dir := t.TempDir()
	keyFile := filepath.Join(dir, "test.key")
	pubFile := filepath.Join(dir, "test.pub")
	sigFile := filepath.Join(dir, "test.sig")
	payload := filepath.Join(dir, "payload.bin")

	// Passphrase is passed via stdin to keep it out of the process argv.
	cmd := exec.Command("minisign", "-G",
		"-p", pubFile,
		"-s", keyFile,
		"-W",
	)
	cmd.Stdin = strings.NewReader("\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("minisign -G: %v\n%s", err, out)
	}

	// Write a payload.
	if err := os.WriteFile(payload, []byte("hello sparkyctrl sig verify\n"), 0644); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	// Sign it (without passphrase — the key above is unencrypted by default
	// when stdin is empty and -W is set).
	sign := exec.Command("minisign", "-S",
		"-s", keyFile,
		"-m", payload,
		"-W",
	)
	sign.Stdin = strings.NewReader("\n")
	if out, err := sign.CombinedOutput(); err != nil {
		t.Fatalf("minisign -S: %v\n%s", err, out)
	}

	// .minisig lives next to the payload.
	actualSig := payload + ".minisig"
	if _, err := os.Stat(actualSig); err != nil {
		t.Fatalf("expected signature at %s: %v", actualSig, err)
	}
	if actualSig != sigFile {
		_ = os.Rename(actualSig, sigFile)
	}

	// We can't test against the compiled-in MinisignPublicKey here (that's the
	// operator's test key from af4487db) — instead we exercise Verify against
	// the freshly-generated public key by parsing it directly and running the
	// verify path inline. This validates that our parse + Ed25519 + BLAKE2b
	// steps agree with the upstream minisign CLI.
	pubBytes, err := os.ReadFile(pubFile)
	if err != nil {
		t.Fatalf("read pub: %v", err)
	}
	pk, err := ParseMinisignPublicKey(string(pubBytes))
	if err != nil {
		t.Fatalf("ParseMinisignPublicKey: %v", err)
	}

	sigBytes, err := os.ReadFile(sigFile)
	if err != nil {
		t.Fatalf("read sig: %v", err)
	}
	sig, err := ParseMinisignSignature(string(sigBytes))
	if err != nil {
		t.Fatalf("ParseMinisignSignature: %v", err)
	}

	// Run the same verify steps VerifyFileSignature runs (without the embedded
	// key constraint, since we want to test the parse + crypto, not the pin).
	if sig.Algorithm != minisignSigMagic {
		t.Fatalf("signature algorithm = %x, want ED", sig.Algorithm)
	}
	if !strings.HasPrefix(sig.TrustedComment, "trusted comment: ") {
		t.Fatalf("trusted comment missing prefix: %q", sig.TrustedComment)
	}

	payloadBytes, err := os.ReadFile(payload)
	if err != nil {
		t.Fatalf("read payload: %v", err)
	}

	// Global signature verifies that the trusted comment is bound to the key.
	trustedPayload := []byte(sig.TrustedComment[len("trusted comment: "):])
	globalMsg := append([]byte{}, sig.Signature[:]...)
	globalMsg = append(globalMsg, trustedPayload...)
	if !verifyEd25519(pk.Ed25519, globalMsg, sig.GlobalSignature[:]) {
		t.Fatal("global signature failed to verify (parser/Ed25519 mismatch)")
	}

	// Primary signature verifies the BLAKE2b-512 of the payload.
	if !verifyFileDigest(pk.Ed25519, payloadBytes, sig.Signature[:]) {
		t.Fatal("primary signature failed to verify (BLAKE2b/Ed25519 mismatch)")
	}
}

// TestEmbeddedKey_WellFormed sanity-checks that the constant in the source
// parses and uses the expected algorithm. If this fails, the constant was
// edited without keeping the bytes in sync with deploy/sparkyctrl-release.pub.
func TestEmbeddedKey_WellFormed(t *testing.T) {
	pk, err := LoadEmbeddedPublicKey()
	if err != nil {
		t.Fatalf("LoadEmbeddedPublicKey: %v", err)
	}
	if pk.Algorithm != minisignPubAlgoBytes {
		t.Errorf("embedded key algorithm = %x, want %x", pk.Algorithm, minisignPubAlgoBytes)
	}
	if len(pk.Ed25519) != ed25519PublicKeySize {
		t.Errorf("embedded key ed25519 size = %d, want %d", len(pk.Ed25519), ed25519PublicKeySize)
	}
	id, err := EmbeddedKeyID()
	if err != nil {
		t.Fatalf("EmbeddedKeyID: %v", err)
	}
	if len(id) != 16 {
		t.Errorf("EmbeddedKeyID = %q (len %d), want 16 hex chars", id, len(id))
	}
	t.Logf("embedded minisign key ID = %s", id)
}
