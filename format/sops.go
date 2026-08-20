package format

import (
	"bytes"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// encPrefix marks a value encrypted by sops. The algorithm is part of the
// payload format, so matching it is far more specific than looking for "ENC".
var encPrefix = []byte("ENC[AES256_GCM,")

// sopsKeySources are the key-management backends sops records in its metadata.
// At least one is present in every encrypted file.
var sopsKeySources = []string{"age", "pgp", "kms", "gcp_kms", "azure_kv", "hc_vault"}

// IsSOPS reports whether the stream is a sops-encrypted document.
//
// sops computes a MAC over the whole document structure, so reformatting an
// encrypted file invalidates it and the file can no longer be decrypted. These
// files are skipped by default; that default is the point.
//
// Detection is structural, not textual, and deliberately so. A file may
// legitimately contain a top-level "sops" key that has nothing to do with
// encryption -- a version-pin file recording which sops CLI to use, for
// instance -- and skipping it would be a silent, confusing no-op. An encrypted
// file is one that either carries sops metadata as a *mapping* with a MAC, or
// contains ENC[AES256_GCM,...] payloads.
func IsSOPS(in []byte) bool {
	// Cheap rejection: almost every file in a repository lands here.
	if !bytes.Contains(in, []byte("sops")) && !bytes.Contains(in, encPrefix) {
		return false
	}
	docs, err := parse(in)
	if err != nil {
		// Unparseable input is not our call to make. The caller reports the
		// parse error; guessing "encrypted" here would hide it.
		return false
	}
	for _, doc := range docs {
		if isSOPSDoc(doc) {
			return true
		}
	}
	return false
}

func isSOPSDoc(doc *yaml.Node) bool {
	return hasSOPSMetadata(doc) || hasEncryptedValue(doc)
}

// hasSOPSMetadata looks for the metadata block sops appends to every file it
// encrypts: a "sops" mapping carrying a MAC and a key source.
func hasSOPSMetadata(doc *yaml.Node) bool {
	root := doc
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return false
	}
	meta := mapValue(root, "sops")
	// A scalar "sops: 3.11.0" is a version pin, not encryption metadata.
	if meta == nil || meta.Kind != yaml.MappingNode {
		return false
	}
	if mapValue(meta, "mac") == nil {
		return false
	}
	for _, src := range sopsKeySources {
		if mapValue(meta, src) != nil {
			return true
		}
	}
	return false
}

// hasEncryptedValue finds sops payloads anywhere in the document, which covers
// encrypted files whose metadata block has been split out or renamed.
func hasEncryptedValue(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	if n.Kind == yaml.ScalarNode && bytes.Contains([]byte(n.Value), encPrefix) {
		return true
	}
	for _, c := range n.Content {
		if hasEncryptedValue(c) {
			return true
		}
	}
	return false
}

// mapValue returns the value node for key in a mapping, or nil.
func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}
