package upgrade

import (
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/blake2b"
)

// MinisignPublicKey is the pinned release-signing public key compiled into the
// binary. It is the only key the upgrade path will ever trust for verifying a
// release artifact — matching the public key shipped at deploy/sparkyctrl-release.pub
// (rotation requires a new release).
//
// The key is encoded in the standard minisign base64 format: 42 bytes total,
//   bytes 0-1   signature algorithm (0x45 0x44 = prehashed Ed25519 / BLAKE2b-512)
//   bytes 2-9   key ID (8 bytes)
//   bytes 10-41 Ed25519 public key (32 bytes)
//
// Generated 2026-06-21 for the af4487db follow-up (signature verification half of
// ticket 87dd3eb1). ROTATE BEFORE PRODUCTION USE — the operator's pinned key
// should replace this before tagging a non-test release. To rotate: generate a
// new keypair with `minisign -G -p new.pub -s new.key`, paste the new base64
// line here AND update deploy/sparkyctrl-release.pub to match (one canonical key
// ID is shipped — installer + Go path share it). Re-tag and re-sign the next
// release with the new key; the old key remains valid for already-shipped
// releases until that release is yanked.
const MinisignPublicKey = "RWTKivm/gay9FW+yJzix4L/oyJ/GfHZHAOrBq2Ed7ko+/53I6GojDKml"

// minisignSigMagic = "ED" (prehashed Ed25519 over BLAKE2b-512).
// Plain (non-prehashed) Ed25519 is "Ed" and not supported here.
var minisignSigMagic = [2]byte{0x45, 0x44}

// minisignKeyAlgoBytes = "ED" — matches what minisign writes into the public key
// file's algorithm field for prehashed Ed25519.
var minisignPubAlgoBytes = [2]byte{0x45, 0x64}

// loadedPublicKey caches the parsed public key on first use.
var loadedPublicKey *minisignPublicKey

// minisignPublicKey is the in-memory parsed form of a minisign public key.
type minisignPublicKey struct {
	Algorithm [2]byte
	KeyID     [8]byte
	Ed25519   ed25519.PublicKey
}

// minisignSignature is the in-memory parsed form of a minisign signature file.
type minisignSignature struct {
	UntrustedComment string
	Algorithm        [2]byte
	KeyID            [8]byte
	Signature        [64]byte
	TrustedComment   string
	GlobalSignature  [64]byte
}

// ParseMinisignPublicKey parses a minisign public key file (the same format
// `minisign -G` writes to disk). The file is two lines:
//   untrusted comment: minisign public key <KEY_ID_HEX>
//   <base64 of 42 bytes>
// Newlines after line 2 are ignored. Carriage returns are tolerated.
func ParseMinisignPublicKey(in string) (*minisignPublicKey, error) {
	lines := strings.SplitN(in, "\n", 2)
	if len(lines) < 2 {
		return nil, errors.New("incomplete minisign public key (expected 2 lines)")
	}
	bin, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[1]))
	if err != nil || len(bin) != 42 {
		return nil, fmt.Errorf("invalid minisign public key encoding: %w", err)
	}
	pk := &minisignPublicKey{}
	copy(pk.Algorithm[:], bin[0:2])
	copy(pk.KeyID[:], bin[2:10])
	pk.Ed25519 = make(ed25519.PublicKey, 32)
	copy(pk.Ed25519, bin[10:42])
	return pk, nil
}

// ParseMinisignSignature parses a .minisig signature file (four lines):
//   untrusted comment: <text>
//   <base64 of 74 bytes>
//   trusted comment: <text>
//   <base64 of 64 bytes>
// Newlines after line 4 are ignored. Carriage returns are tolerated.
func ParseMinisignSignature(in string) (*minisignSignature, error) {
	lines := strings.SplitN(in, "\n", 4)
	if len(lines) < 4 {
		return nil, errors.New("incomplete minisign signature (expected 4 lines)")
	}
	sig := &minisignSignature{
		UntrustedComment: strings.TrimRight(lines[0], "\r"),
		TrustedComment:   strings.TrimRight(lines[2], "\r"),
	}
	bin1, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[1]))
	if err != nil || len(bin1) != 74 {
		return nil, fmt.Errorf("invalid minisign signature line 2: %w", err)
	}
	copy(sig.Algorithm[:], bin1[0:2])
	copy(sig.KeyID[:], bin1[2:10])
	copy(sig.Signature[:], bin1[10:74])
	bin2, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[3]))
	if err != nil || len(bin2) != 64 {
		return nil, fmt.Errorf("invalid minisign signature line 4: %w", err)
	}
	copy(sig.GlobalSignature[:], bin2)
	return sig, nil
}

// LoadEmbeddedPublicKey parses the compiled-in MinisignPublicKey constant. It
// returns an error if the constant is malformed or has the wrong algorithm —
// which would mean the constant was edited without re-deriving the key bytes.
// The result is cached so the parse cost is paid once per process.
func LoadEmbeddedPublicKey() (*minisignPublicKey, error) {
	if loadedPublicKey != nil {
		return loadedPublicKey, nil
	}
	pk, err := ParseMinisignPublicKey("untrusted comment: minisign public key\n" + MinisignPublicKey)
	if err != nil {
		return nil, fmt.Errorf("embedded minisign public key is malformed (constant corrupted?): %w", err)
	}
	if pk.Algorithm != minisignPubAlgoBytes {
		return nil, fmt.Errorf("embedded minisign public key uses unsupported algorithm %x (need ED)", pk.Algorithm)
	}
	loadedPublicKey = pk
	return pk, nil
}

// verifyMinisignSignatureAgainstKey runs the two-step minisign verify against
// an arbitrary public key (no pinning). Returns nil if both the global
// signature (trusted-comment binding) and the primary signature (file digest)
// check out. This is the inner verifier — VerifyFileSignature just glues it
// to the embedded key and adds the pin check.
//
// Split out so the test suite can exercise the parser + Ed25519 + BLAKE2b
// logic against any minisign-generated file without needing to recompile with
// a different pinned key.
func verifyMinisignSignatureAgainstKey(pk *minisignPublicKey, fileBytes []byte, sig *minisignSignature) error {
	if sig.Algorithm != minisignSigMagic {
		return fmt.Errorf("minisign signature uses unsupported algorithm %x (need ED / prehashed Ed25519)", sig.Algorithm)
	}
	if !strings.HasPrefix(sig.TrustedComment, "trusted comment: ") {
		return errors.New("minisign trusted comment missing expected prefix")
	}

	// Step 1: global signature binds the trusted comment to the key. The signed
	// message is signature_bytes || trusted_comment_payload (everything after
	// the "trusted comment: " prefix — the key id, hash algo, timestamp, etc.).
	trustedPayload := []byte(sig.TrustedComment[len("trusted comment: "):])
	globalMsg := make([]byte, 0, 64+len(trustedPayload))
	globalMsg = append(globalMsg, sig.Signature[:]...)
	globalMsg = append(globalMsg, trustedPayload...)
	if !ed25519.Verify(pk.Ed25519, globalMsg, sig.GlobalSignature[:]) {
		return errors.New("minisign global signature invalid (trusted comment not bound to key)")
	}

	// Step 2: primary signature covers BLAKE2b-512(file).
	h, _ := blake2b.New512(nil)
	h.Write(fileBytes)
	digest := h.Sum(nil)
	if !ed25519.Verify(pk.Ed25519, digest, sig.Signature[:]) {
		return errors.New("minisign signature does not match file content")
	}
	return nil
}

// verifyFileDigest is exposed for the test suite — it runs ONLY the primary
// signature check (BLAKE2b-512 of file against Ed25519). The test exercises
// this directly with a freshly generated key to prove the crypto path matches
// what the upstream `minisign` CLI produces, independent of the embedded pin.
func verifyFileDigest(pub ed25519.PublicKey, fileBytes, sig []byte) bool {
	h, _ := blake2b.New512(nil)
	h.Write(fileBytes)
	return ed25519.Verify(pub, h.Sum(nil), sig)
}

// verifyEd25519 is the raw Ed25519 verify — exposed for the test suite so the
// global-signature path can be checked against a known key.
func verifyEd25519(pub ed25519.PublicKey, msg, sig []byte) bool {
	return ed25519.Verify(pub, msg, sig)
}

// ed25519PublicKeySize is the Ed25519 public-key length in bytes (32).
// Exposed as a const so the test can assert the parsed key has the right size
// without importing the crypto/ed25519 size into test code.
const ed25519PublicKeySize = 32

// VerifyFileSignature verifies that sigText is a valid minisign signature for
// the file at path. The signature is checked against LoadEmbeddedPublicKey —
// there is no per-call key argument because the whole point of pinning is that
// only the compiled-in key is accepted.
//
// Defense in depth:
//  1. The signature key ID must match the embedded key ID (constant-time
//     compare so the comparison doesn't leak which bytes differ).
//  2. The "global signature" (which binds the trusted comment to the key) is
//     verified. This blocks key-substitution attacks where an attacker re-signs
//     the file with a different key but tries to claim the old signature is
//     still valid.
//  3. The file hash (BLAKE2b-512) is checked against the primary Ed25519
//     signature. This is what proves the file came from the key holder.
//  4. Both signatures must verify — one without the other is rejected.
func VerifyFileSignature(path, sigText string) error {
	pk, err := LoadEmbeddedPublicKey()
	if err != nil {
		return err
	}
	sig, err := ParseMinisignSignature(sigText)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(sig.KeyID[:], pk.KeyID[:]) != 1 {
		return fmt.Errorf("minisign signature key ID %x does not match pinned key ID %x (key rotated?)", sig.KeyID, pk.KeyID)
	}

	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read signed file: %w", err)
	}
	return verifyMinisignSignatureAgainstKey(pk, fileBytes, sig)
}

// EmbeddedKeyID returns the 8-byte key ID of the compiled-in public key, hex-
// encoded (uppercase). Useful for logging which key verified a release.
func EmbeddedKeyID() (string, error) {
	pk, err := LoadEmbeddedPublicKey()
	if err != nil {
		return "", err
	}
	const hex = "0123456789ABCDEF"
	out := make([]byte, 16)
	for i, b := range pk.KeyID {
		out[i*2] = hex[b>>4]
		out[i*2+1] = hex[b&0x0f]
	}
	return string(out), nil
}

// SetEmbeddedPublicKeyForTest replaces the pinned public key with the supplied
// minisign-encoded base64 string (the second line of a .pub file). Returns a
// restore function that puts the production key back. ONLY for tests — never
// call this from non-test code paths.
//
// The test path is the right one because the production key's private
// counterpart isn't shipped with the repo (the security model depends on
// that). Tests that need to exercise the verify path end-to-end generate a
// fresh keypair with `minisign -G`, install it here, sign their test
// payloads with the private key, then restore.
func SetEmbeddedPublicKeyForTest(pubKeyBase64 string) (restore func(), err error) {
	prev := loadedPublicKey
	pk, err := ParseMinisignPublicKey("untrusted comment: minisign public key\n" + pubKeyBase64)
	if err != nil {
		return func() {}, fmt.Errorf("test pub key parse: %w", err)
	}
	if pk.Algorithm != minisignPubAlgoBytes {
		return func() {}, fmt.Errorf("test pub key has unsupported algorithm %x", pk.Algorithm)
	}
	loadedPublicKey = pk
	return func() { loadedPublicKey = prev }, nil
}

// VerifyStringSignature is the in-memory counterpart to VerifyFileSignature —
// it accepts the signed payload as a byte slice instead of reading it from
// disk. Used by DownloadAndVerify to verify the SHA256SUMS body (which is
// small and is already in memory by the time we want to check it).
func VerifyStringSignature(payload []byte, sigText string) error {
	pk, err := LoadEmbeddedPublicKey()
	if err != nil {
		return err
	}
	sig, err := ParseMinisignSignature(sigText)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(sig.KeyID[:], pk.KeyID[:]) != 1 {
		return fmt.Errorf("minisign signature key ID %x does not match pinned key ID %x (key rotated?)", sig.KeyID, pk.KeyID)
	}
	return verifyMinisignSignatureAgainstKey(pk, payload, sig)
}

// verifyStringSignature is the package-internal alias — DownloadAndVerify
// already has a string of sums from fetchString and we don't want to expose
// the underlying []byte conversion at every call site.
func verifyStringSignature(payload string, sigText string) error {
	return VerifyStringSignature([]byte(payload), sigText)
}
