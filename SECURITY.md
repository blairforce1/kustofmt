# Security policy

## Reporting a vulnerability

Please report security issues privately through GitHub's private vulnerability
reporting: open the [Security tab](https://github.com/blairforce1/kustofmt/security/advisories/new)
and file a draft advisory. Please do not open a public issue for a
security-relevant defect.

You should get an acknowledgement within 7 days. If a fix is warranted it will
be released as a patch version, credited in the changelog, and the advisory
published.

## Supported versions

| Version | Supported |
|---------|-----------|
| latest minor | ✅ |
| anything older | ❌ |

Fixes land on the latest minor release. There are no long-term support
branches; upgrade to the current minor to receive them.

## Threat model

kustofmt reads YAML files and writes YAML files. It makes no network calls,
executes nothing, and reads no configuration file. The realistic risks are:

- **Malformed input causing a crash or hang.** The parser is fuzzed
  continuously in CI (`make fuzz`); please report anything that panics or fails
  to terminate.
- **Silent corruption of a file it rewrites.** This is the serious one.
  `Format` verifies its own output before returning it: the result must decode
  to exactly the same values as the input, and must be a fixed point. If you
  can make `-w` change a document's meaning, that is a security-relevant bug in
  this tool's terms, whatever the YAML specification says. Please report it.
- **Reformatting an encrypted file.** sops computes a MAC over the document
  structure, so reformatting breaks decryption. Encrypted files are detected
  and skipped by default. A false negative in that detection is a reportable
  bug; `--include-sops` deliberately disables the protection.
