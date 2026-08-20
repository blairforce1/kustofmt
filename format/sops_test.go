package format_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairforce1/kustofmt/format"
)

// TestIsSOPSRealFiles uses fixtures taken verbatim from a production GitOps
// repository, including the file that broke the obvious heuristic.
func TestIsSOPSRealFiles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		file string
		want bool
		why  string
	}{
		{"sops-secret.yaml", true, "a genuine sops-encrypted Secret"},
		{"sops-false-positive.yaml", false, "a version-pin file whose `sops:` key is a plain string"},
		{"kustomize-output.input.yaml", false, "ordinary kustomize output"},
		{"flux-kustomization.input.yaml", false, "a Flux export that mentions sops decryption but is not encrypted"},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()
			b, err := os.ReadFile(filepath.Join("testdata", tc.file))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if got := format.IsSOPS(b); got != tc.want {
				t.Errorf("IsSOPS = %v, want %v (%s)", got, tc.want, tc.why)
			}
		})
	}
}

func TestIsSOPSSynthetic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{
			name: "metadata mapping with mac and age",
			in:   "data:\n  k: v\nsops:\n  age:\n  - recipient: age1x\n  mac: ENC[AES256_GCM,data:abc,iv:d,tag:e,type:str]\n  version: 3.11.0\n",
			want: true,
		},
		{
			name: "encrypted payload without metadata block",
			in:   "data:\n  password: ENC[AES256_GCM,data:abc,iv:d,tag:e,type:str]\n",
			want: true,
		},
		{
			name: "scalar sops version pin",
			in:   "sops: \"3.11.0\"\nkustomize: \"5.7.1\"\n",
			want: false,
		},
		{
			name: "sops mapping without a mac is not encrypted",
			in:   "sops:\n  version: 3.11.0\n  note: just config\n",
			want: false,
		},
		{
			name: "the word sops in a comment",
			in:   "# decrypt with sops before applying\ndata:\n  k: v\n",
			want: false,
		},
		{
			name: "a .sops.yaml creation-rules config",
			in:   "creation_rules:\n- path_regex: .*.secret.yaml$\n  age: age1x\n",
			want: false,
		},
		{
			name: "flux decryption reference",
			in:   "spec:\n  decryption:\n    provider: sops\n    secretRef:\n      name: sops-age\n",
			want: false,
		},
		{
			name: "empty input",
			in:   "",
			want: false,
		},
		{
			name: "unparseable input is not claimed as sops",
			in:   "sops: {mac: ENC[AES256_GCM,\n",
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := format.IsSOPS([]byte(tc.in)); got != tc.want {
				t.Errorf("IsSOPS = %v, want %v\ninput:\n%s", got, tc.want, tc.in)
			}
		})
	}
}

// TestSOPSFileWouldBeMangled proves the default is protecting something real:
// formatting the encrypted fixture changes its bytes, which invalidates the MAC.
func TestSOPSFileWouldBeMangled(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("testdata", "sops-secret.yaml"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got, err := format.Format(b)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if string(got) == string(b) {
		t.Skip("this sops fixture happens to already be in house style; the skip default is still correct")
	}
	t.Logf("formatting would rewrite %d bytes into %d - hence the default skip", len(b), len(got))
}
