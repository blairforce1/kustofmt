// Package compat models kustofmt's kustomize compatibility matrix.
//
// kustofmt's output style is whatever kyaml's emitter produces, so every
// kustofmt release is built against exactly one kyaml version. A repository
// that renders with a given kustomize CLI can therefore pin the kustofmt built
// from the same library, and the claim is checkable rather than asserted.
//
// The matrix records that mapping. It is the single source of truth: the tables
// in README.md and CHANGELOG.md are generated from it, and CI re-derives every
// row from upstream rather than trusting the file.
package compat

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// Release is one kustofmt release and the kustomize releases it matches.
type Release struct {
	// Kustofmt is the kustofmt version, without a leading "v".
	Kustofmt string `yaml:"kustofmt"`
	// Kyaml is the kyaml version this release links, with a leading "v".
	Kyaml string `yaml:"kyaml"`
	// Kustomize lists every kustomize CLI release shipping exactly this kyaml.
	Kustomize []string `yaml:"kustomize"`
}

// Matrix is the whole compatibility file.
type Matrix struct {
	// Version is the kustofmt release this tree would publish, and the number
	// the release workflow tags.
	//
	// It is deliberately not derived from the newest row. Most releases are
	// driven by kyaml and do add a row, but a change with no kyaml behind it --
	// a packaging or signing change -- has no row to add, because one row per
	// kyaml is an invariant validate enforces. Without a version of its own,
	// such a change could never be released at all.
	Version string `yaml:"version"`
	// Floor is the oldest kustomize release tracked. Anything older is
	// deliberately absent rather than accidentally missing, and the gate needs
	// to know the difference.
	Floor string `yaml:"floor"`
	// Releases is ordered oldest kustofmt release first.
	Releases []Release `yaml:"releases"`
}

// Load reads a matrix from disk.
func Load(path string) (*Matrix, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Matrix
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := m.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &m, nil
}

// validate rejects a matrix that cannot be true, so a typo fails at load rather
// than becoming a wrong answer later.
func (m *Matrix) validate() error {
	if m.Floor == "" {
		return errors.New("floor is required")
	}
	if m.Version == "" {
		return errors.New("version is required")
	}
	if strings.HasPrefix(m.Version, "v") {
		return fmt.Errorf("version %q should not carry a leading v", m.Version)
	}
	if len(m.Releases) == 0 {
		return errors.New("no releases recorded")
	}
	seenKyaml := map[string]string{}
	seenKustomize := map[string]string{}
	for _, r := range m.Releases {
		if r.Kustofmt == "" || r.Kyaml == "" {
			return fmt.Errorf("release %+v is missing kustofmt or kyaml", r)
		}
		if !strings.HasPrefix(r.Kyaml, "v") {
			return fmt.Errorf("kyaml %q should carry a leading v", r.Kyaml)
		}
		if strings.HasPrefix(r.Kustofmt, "v") {
			return fmt.Errorf("kustofmt %q should not carry a leading v", r.Kustofmt)
		}
		// One kyaml per release is the whole premise: two rows sharing a kyaml
		// would make "which kustofmt do I pin" ambiguous.
		if prev, dup := seenKyaml[r.Kyaml]; dup {
			return fmt.Errorf("kyaml %s appears in both %s and %s", r.Kyaml, prev, r.Kustofmt)
		}
		seenKyaml[r.Kyaml] = r.Kustofmt
		for _, k := range r.Kustomize {
			if prev, dup := seenKustomize[k]; dup {
				return fmt.Errorf("kustomize %s appears in both %s and %s", k, prev, r.Kustofmt)
			}
			seenKustomize[k] = r.Kustofmt
		}
	}
	// The two halves must not drift. A row added without advancing the version
	// would release under a number that is already published; a version behind
	// the rows would release a kyaml bump under the previous number. Neither is
	// visible by reading the file, so it fails at load instead.
	if CompareVersions(m.Version, m.Current().Kustofmt) < 0 {
		return fmt.Errorf("version %s is behind the newest release row (%s)", m.Version, m.Current().Kustofmt)
	}
	return nil
}

// Current returns the newest release, which is the one go.mod must match.
func (m *Matrix) Current() Release {
	return m.Releases[len(m.Releases)-1]
}

// ByKyaml finds the release built against a kyaml version.
func (m *Matrix) ByKyaml(kyaml string) (Release, bool) {
	for _, r := range m.Releases {
		if r.Kyaml == kyaml {
			return r, true
		}
	}
	return Release{}, false
}

// HasKustomize reports whether a kustomize release is already recorded.
func (m *Matrix) HasKustomize(version string) bool {
	for _, r := range m.Releases {
		for _, k := range r.Kustomize {
			if k == version {
				return true
			}
		}
	}
	return false
}

// Action is what a newly published kustomize release requires of us.
type Action string

const (
	// ActionNone means the release is already recorded.
	ActionNone Action = "no-op"
	// ActionMatrixOnly means an existing kustofmt release already links this
	// kyaml, so nothing is rebuilt: the release just joins that row.
	ActionMatrixOnly Action = "matrix-only"
	// ActionRebuild means no release links this kyaml yet, so kustofmt must be
	// rebuilt against it and a new version cut.
	ActionRebuild Action = "rebuild"
)

// Decision is the outcome of examining one kustomize release.
type Decision struct {
	Kustomize string
	Kyaml     string
	Action    Action
	// Target is the kustofmt version affected: the existing one for
	// ActionMatrixOnly and ActionNone, the proposed new one for ActionRebuild.
	Target string
}

// Decide works out what a kustomize release requires, given the kyaml it links.
//
// The middle case also covers a kustomize release that *downgrades* kyaml back
// to one already shipped: it joins that release's row rather than inventing a
// new version for a library kustofmt has already been built against.
func (m *Matrix) Decide(kustomizeVersion, kyamlVersion string) Decision {
	d := Decision{Kustomize: kustomizeVersion, Kyaml: kyamlVersion}
	switch {
	case m.HasKustomize(kustomizeVersion):
		d.Action = ActionNone
		if r, ok := m.ByKyaml(kyamlVersion); ok {
			d.Target = r.Kustofmt
		}
	default:
		if r, ok := m.ByKyaml(kyamlVersion); ok {
			d.Action = ActionMatrixOnly
			d.Target = r.Kustofmt
			return d
		}
		d.Action = ActionRebuild
		d.Target = NextPatch(m.Version)
	}
	return d
}

// NextPatch increments the patch component.
//
// A kyaml bump that leaves the golden corpus untouched changes nothing a user
// can observe, and the output style is this tool's public API -- so it is a
// patch. A bump that *does* move the goldens is a style change, and the caller
// stops for review rather than reaching for this.
func NextPatch(version string) string {
	major, minor, patch, err := splitVersion(version)
	if err != nil {
		return version
	}
	return fmt.Sprintf("%d.%d.%d", major, minor, patch+1)
}

// NextMinor increments the minor component and resets the patch, for a release
// whose emitted style changed.
func NextMinor(version string) string {
	major, minor, _, err := splitVersion(version)
	if err != nil {
		return version
	}
	return fmt.Sprintf("%d.%d.0", major, minor+1)
}

func splitVersion(version string) (major, minor, patch int, err error) {
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("not a three-part version: %q", version)
	}
	nums := make([]int, 3)
	for i, p := range parts {
		nums[i], err = strconv.Atoi(p)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("not a version: %q", version)
		}
	}
	return nums[0], nums[1], nums[2], nil
}

// CompareVersions orders dotted numeric versions, returning -1, 0 or 1.
// Sorting these as strings puts 5.10.0 before 5.9.0, which is how a matrix ends
// up quietly out of order.
func CompareVersions(a, b string) int {
	as := strings.Split(strings.TrimPrefix(a, "v"), ".")
	bs := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var ai, bi int
		if i < len(as) {
			ai, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bi, _ = strconv.Atoi(bs[i])
		}
		if ai != bi {
			if ai < bi {
				return -1
			}
			return 1
		}
	}
	return 0
}

// Record applies a decision to the in-memory matrix.
//
// Both the real apply path and the status forecast go through here, so a
// forecast cannot drift from what applying would actually do. Without it,
// status evaluates every pending release against the *current* matrix and
// predicts the same version for all of them, which is wrong the moment more
// than one is outstanding.
func (m *Matrix) Record(d Decision) {
	switch d.Action {
	case ActionNone:
		return
	case ActionMatrixOnly:
		for i := range m.Releases {
			if m.Releases[i].Kustofmt == d.Target {
				m.Releases[i].Kustomize = append(m.Releases[i].Kustomize, d.Kustomize)
				return
			}
		}
	case ActionRebuild:
		m.Releases = append(m.Releases, Release{
			Kustofmt:  d.Target,
			Kyaml:     d.Kyaml,
			Kustomize: []string{d.Kustomize},
		})
		// Advancing the version here, rather than in the caller, is what keeps
		// the forecast honest: status and apply both reach this line.
		m.Version = d.Target
	}
}
