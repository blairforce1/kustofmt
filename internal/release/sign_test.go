// Package release holds no code. It exists for this test, which guards the one
// part of the release pipeline CI otherwise never executes: the cosign
// invocation in .goreleaser.yaml.
//
// That gap is not theoretical. cosign v3 removed --output-certificate and
// --output-signature; passing them is accepted, warned about and ignored, so
// cosign exits 0 having written nothing. GoReleaser does not help: its sign
// pipe registers the signature and certificate as artifacts to upload without
// ever checking the files exist, so the first thing to notice is the release
// upload, by which point the tag is cut and the image is pushed.
//
// The test reads the arguments out of .goreleaser.yaml rather than restating
// them. A test with the arguments copied into it would have gone on passing
// while the config said --output-certificate, which is the failure it is here
// to catch.
package release_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// signConfig mirrors the fields of a `signs` entry that decide what lands on
// disk. GoReleaser has many more; these are the ones with a file behind them.
type signConfig struct {
	Cmd         string   `yaml:"cmd"`
	Signature   string   `yaml:"signature"`
	Certificate string   `yaml:"certificate"`
	Args        []string `yaml:"args"`
	Artifacts   string   `yaml:"artifacts"`
}

type goreleaserConfig struct {
	Signs []signConfig `yaml:"signs"`
}

const (
	configPath   = "../../.goreleaser.yaml"
	workflowPath = "../../.github/workflows/release.yaml"
)

// pinnedCosign reads the cosign version the release workflow installs. Reading
// it rather than restating it means this test and the release cannot drift, and
// it matters: cosign's flags move between majors, so a test run against a
// different cosign proves nothing about the release.
func pinnedCosign(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^\s*cosign-release:\s*(v\S+)\s*$`).FindSubmatch(raw)
	if m == nil {
		t.Fatalf("%s: no cosign-release pin found; the release installs an unpinned cosign", workflowPath)
	}
	return string(m[1])
}

// installedCosign returns the version of the cosign on PATH.
func installedCosign(t *testing.T, cosign string) string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), cosign, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("cosign version: %v\n%s", err, out)
	}
	m := regexp.MustCompile(`GitVersion:\s*(v\S+)`).FindSubmatch(out)
	if m == nil {
		t.Fatalf("cannot read a version out of:\n%s", out)
	}
	return string(m[1])
}

func loadSignConfig(t *testing.T) signConfig {
	t.Helper()
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg goreleaserConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("%s: %v", configPath, err)
	}
	if len(cfg.Signs) != 1 {
		t.Fatalf("%s: got %d signs entries, want exactly 1", configPath, len(cfg.Signs))
	}
	s := cfg.Signs[0]
	if s.Cmd != "cosign" {
		t.Fatalf("signs[0].cmd = %q, want cosign", s.Cmd)
	}
	// A separate certificate file only exists in the keyless flow, which this
	// test cannot exercise, and the flags that wrote one were removed in cosign
	// v3. The bundle carries the certificate inside it instead. Refusing here
	// is deliberate: it stops --output-certificate coming back through a config
	// change that nothing else would catch.
	if s.Certificate != "" {
		t.Fatalf("signs[0].certificate is set to %q; the bundle carries the certificate, "+
			"and cosign v3 removed the flags that write a separate one", s.Certificate)
	}
	// GoReleaser's own default, applied when the field is absent. Mirroring it
	// keeps the test honest about what would actually run.
	if s.Signature == "" {
		s.Signature = "${artifact}.sig"
	}
	return s
}

// TestSigningWritesWhatTheConfigDeclares runs the configured cosign command
// against a throwaway key -- the keyless path needs an OIDC identity, which
// only exists in CI -- and holds it to the contract GoReleaser assumes: every
// file the config names is on disk afterwards.
func TestSigningWritesWhatTheConfigDeclares(t *testing.T) {
	if testing.Short() {
		t.Skip("shells out to cosign")
	}
	cosign, err := exec.LookPath("cosign")
	if err != nil {
		t.Skip("cosign is not installed; see the Signing job in ci.yaml")
	}
	// Skipping rather than failing mirrors how make shellcheck treats a system
	// binary that is not the pinned one: a laptop with the wrong version should
	// say so, not produce a verdict CI will contradict.
	if want, got := pinnedCosign(t), installedCosign(t, cosign); got != want {
		t.Skipf("cosign %s is installed but the release pins %s; this test only speaks for the pinned one", got, want)
	}
	cfg := loadSignConfig(t)

	dir := t.TempDir()
	artifact := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(artifact, []byte("d41d8cd98f00b204e9800998ecf8427e  kustofmt\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// An unencrypted throwaway key, generated per-run and thrown away with the
	// temp directory.
	env := append(os.Environ(), "COSIGN_PASSWORD=")
	keygen := exec.CommandContext(t.Context(), cosign, "generate-key-pair")
	keygen.Dir = dir
	keygen.Env = env
	if out, err := keygen.CombinedOutput(); err != nil {
		t.Fatalf("generate-key-pair: %v\n%s", err, out)
	}

	expand := strings.NewReplacer(
		"${artifact}", artifact,
		"${signature}", substitute(cfg.Signature, artifact),
	)
	args := make([]string, 0, len(cfg.Args)+4)
	for _, a := range cfg.Args {
		args = append(args, expand.Replace(a))
	}
	// Swap the keyless identity for the throwaway key. Everything the config
	// says about *what is written where* is left exactly as it is.
	args = append(args, "--key", filepath.Join(dir, "cosign.key"), "--tlog-upload=false")

	cmd := exec.CommandContext(t.Context(), cosign, args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cosign %s: %v\n%s", strings.Join(args, " "), err, out)
	}

	signature := substitute(cfg.Signature, artifact)
	t.Run("the declared signature exists", func(t *testing.T) {
		// The exact assertion cosign v3 fails against the old flags: it exits 0
		// and writes nothing.
		info, err := os.Stat(signature)
		if err != nil {
			t.Fatalf("config names %s but cosign did not write it (exit 0)\ncosign output:\n%s",
				filepath.Base(signature), out)
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty", filepath.Base(signature))
		}
	})

	t.Run("the signature is a sigstore bundle", func(t *testing.T) {
		body, err := os.ReadFile(signature)
		if err != nil {
			t.Skip("nothing to inspect; the previous subtest says why")
		}
		// The bundle carries the inclusion proof with it, which is what lets
		// verification skip the Rekor lookup the detached pair forces.
		if !strings.Contains(string(body), "sigstore.bundle") {
			t.Errorf("%s is not a Sigstore bundle; got:\n%.200s", filepath.Base(signature), body)
		}
	})

	t.Run("verifies", func(t *testing.T) {
		verify := exec.CommandContext(t.Context(), cosign, "verify-blob",
			"--key", filepath.Join(dir, "cosign.pub"),
			"--bundle", signature,
			"--insecure-ignore-tlog", // this key never reached a transparency log
			artifact,
		)
		verify.Dir = dir
		verify.Env = env
		if out, err := verify.CombinedOutput(); err != nil {
			t.Errorf("verify-blob: %v\n%s", err, out)
		}
	})
}

// substitute resolves ${artifact} inside a filename template. GoReleaser
// resolves these relative to its dist directory; here the artifact's own
// directory plays that part.
func substitute(template, artifact string) string {
	if template == "" {
		return ""
	}
	return strings.ReplaceAll(template, "${artifact}", artifact)
}
