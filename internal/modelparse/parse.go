package modelparse

import (
	"regexp"
	"strconv"
	"strings"
)

// Parsed is a structured view of an Ollama model name.
type Parsed struct {
	Raw       string
	Namespace string // empty or "library" or user namespace
	Name      string // full model name without tag (may include namespace path without host)
	BaseName  string // name without namespace, e.g. qwen2.5-coder
	Tag       string
	Family    string  // brand stem, e.g. qwen
	Version   Version // series version from name
	Specialty string  // coder, vl, math, ...
	SizeClass string  // e.g. 32b
}

// Version is a dotted numeric version for ranking generations.
type Version struct {
	Parts []int
	Raw   string
}

var (
	sizeRe       = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)(b|m)\b`)
	versionRe    = regexp.MustCompile(`(?i)(\d+(?:\.\d+)*)`)
	specialties  = []string{"coder", "code", "vl", "vision", "math", "instruct", "chat", "embedding", "embed", "guard", "reasoning", "r1"}
	quantInTagRe = regexp.MustCompile(`(?i)(q\d+|fp\d+|f\d+|iq\d)`)
)

// Parse splits model:tag into structured fields.
// parameterSize is optional (e.g. "32.0B" from /api/tags details).
func Parse(full string, parameterSize string) Parsed {
	full = strings.TrimSpace(full)
	p := Parsed{Raw: full, Tag: "latest"}

	namePart := full
	if i := strings.LastIndex(full, ":"); i >= 0 {
		// avoid treating host:port as tag â€” only split if looks like model:tag
		maybeTag := full[i+1:]
		if !strings.Contains(maybeTag, "/") && maybeTag != "" {
			namePart = full[:i]
			p.Tag = maybeTag
		}
	}

	p.Name = namePart
	// namespace/model
	if strings.Contains(namePart, "/") {
		parts := strings.SplitN(namePart, "/", 2)
		p.Namespace = parts[0]
		p.BaseName = parts[1]
	} else {
		p.Namespace = "library"
		p.BaseName = namePart
	}

	p.Specialty = detectSpecialty(p.BaseName)
	p.Family, p.Version = detectFamilyVersion(p.BaseName)
	p.SizeClass = detectSize(p.Tag, parameterSize)

	return p
}

func detectSpecialty(base string) string {
	lower := strings.ToLower(base)
	// prefer longer / more specific tokens first (already ordered)
	for _, s := range specialties {
		if strings.Contains(lower, s) {
			if s == "code" {
				return "coder"
			}
			if s == "vision" {
				return "vl"
			}
			if s == "embed" {
				return "embedding"
			}
			return s
		}
	}
	return ""
}

func detectFamilyVersion(base string) (string, Version) {
	// strip specialty suffixes for cleaner parse
	work := strings.ToLower(base)
	for _, s := range specialties {
		work = strings.ReplaceAll(work, "-"+s, "")
		work = strings.ReplaceAll(work, s, "")
	}
	work = strings.Trim(work, "-_.")

	// leading letters = family
	family := ""
	i := 0
	for i < len(work) && (work[i] >= 'a' && work[i] <= 'z') {
		family += string(work[i])
		i++
	}
	if family == "" {
		family = strings.Split(work, "-")[0]
		// strip digits from family token
		var b strings.Builder
		for _, r := range family {
			if r >= 'a' && r <= 'z' {
				b.WriteRune(r)
			}
		}
		family = b.String()
	}

	// version: first numeric sequence in base (prefer after family)
	rest := work
	if i < len(work) {
		rest = work[i:]
	}
	ver := Version{}
	if m := versionRe.FindString(rest); m != "" {
		ver = ParseVersion(m)
	} else if m := versionRe.FindString(strings.ToLower(base)); m != "" {
		// fallback whole base
		ver = ParseVersion(m)
	}
	return family, ver
}

// ParseVersion parses "2.5" / "3" / "3.1.2" into parts.
func ParseVersion(s string) Version {
	s = strings.TrimSpace(s)
	v := Version{Raw: s}
	for _, part := range strings.Split(s, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			// try float-ish single part already handled by split
			if f, err2 := strconv.ParseFloat(part, 64); err2 == nil {
				v.Parts = append(v.Parts, int(f))
			}
			continue
		}
		v.Parts = append(v.Parts, n)
	}
	return v
}

// Compare returns -1 if v < other, 0 if equal, 1 if v > other.
func (v Version) Compare(other Version) int {
	max := len(v.Parts)
	if len(other.Parts) > max {
		max = len(other.Parts)
	}
	for i := 0; i < max; i++ {
		a, b := 0, 0
		if i < len(v.Parts) {
			a = v.Parts[i]
		}
		if i < len(other.Parts) {
			b = other.Parts[i]
		}
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
	}
	return 0
}

func (v Version) IsZero() bool {
	return len(v.Parts) == 0
}

// String returns dotted version or empty.
func (v Version) String() string {
	if v.Raw != "" {
		return v.Raw
	}
	if len(v.Parts) == 0 {
		return ""
	}
	parts := make([]string, len(v.Parts))
	for i, p := range v.Parts {
		parts[i] = strconv.Itoa(p)
	}
	return strings.Join(parts, ".")
}

func detectSize(tag, parameterSize string) string {
	// Prefer pure size tags (7b, 30b). Composite/exotic tags like e4b: prefer parameter_size.
	if tagLooksLikePureSize(tag) {
		if s := sizeFromString(tag); s != "" {
			return s
		}
	}
	if s := sizeFromString(parameterSize); s != "" {
		return s
	}
	if s := sizeFromString(tag); s != "" {
		return s
	}
	return ""
}

func tagLooksLikePureSize(tag string) bool {
	t := strings.ToLower(strings.TrimSpace(tag))
	return sizeRe.MatchString(t) && !strings.ContainsAny(t, "-_") &&
		(strings.HasSuffix(t, "b") || strings.HasSuffix(t, "m")) &&
		(t[0] >= '0' && t[0] <= '9')
}

func sizeFromString(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "latest") {
		return ""
	}
	// strip quant suffixes for matching: 32b-instruct-q4_K_M â†’ 32b
	m := sizeRe.FindStringSubmatch(strings.ToLower(s))
	if m == nil {
		return ""
	}
	num := m[1]
	unit := m[2]
	// normalize 32.0 â†’ 32
	if strings.Contains(num, ".") {
		if f, err := strconv.ParseFloat(num, 64); err == nil {
			if f == float64(int(f)) {
				num = strconv.Itoa(int(f))
			}
		}
	}
	return num + unit
}

// SizeCompatible reports exact match or same weight class.
// For large models (â‰¥7B), allows ~15% relative difference (min Â±2B) so
// e.g. 32b â‰ˆ 30b counts as a notional same-weight upgrade.
func SizeCompatible(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	na, ua, oka := splitSize(a)
	nb, ub, okb := splitSize(b)
	if !oka || !okb || ua != ub {
		return false
	}
	diff := na - nb
	if diff < 0 {
		diff = -diff
	}
	// tiny models: exact or Â±0.5
	if na < 7 || nb < 7 {
		return diff <= 0.5
	}
	// large models: within 15% of the larger size, at least 2B slack
	larger := na
	if nb > larger {
		larger = nb
	}
	slack := larger * 0.15
	if slack < 2 {
		slack = 2
	}
	return diff <= slack
}

func splitSize(s string) (float64, string, bool) {
	m := sizeRe.FindStringSubmatch(strings.ToLower(s))
	if m == nil {
		return 0, "", false
	}
	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, "", false
	}
	return f, m[2], true
}

// LibraryURL returns the ollama.com library page for this model.
func (p Parsed) LibraryURL() string {
	name := p.BaseName
	if p.Namespace != "" && p.Namespace != "library" {
		name = p.Namespace + "/" + p.BaseName
	}
	if p.Tag != "" && p.Tag != "latest" {
		return "https://ollama.com/library/" + name + ":" + p.Tag
	}
	return "https://ollama.com/library/" + name
}

// RegistryPath returns namespace/name for registry API (library/foo or user/foo).
func (p Parsed) RegistryPath() string {
	if p.Namespace != "" && p.Namespace != "library" {
		return p.Namespace + "/" + p.BaseName
	}
	return "library/" + p.BaseName
}

// FullName returns name:tag.
func (p Parsed) FullName() string {
	name := p.BaseName
	if p.Namespace != "" && p.Namespace != "library" {
		name = p.Namespace + "/" + p.BaseName
	}
	return name + ":" + p.Tag
}

// IsQuantHeavyTag is true for tags that are mostly quant identifiers.
func IsQuantHeavyTag(tag string) bool {
	t := strings.ToLower(tag)
	if sizeRe.MatchString(t) && !quantInTagRe.MatchString(t) {
		return false
	}
	return quantInTagRe.MatchString(t)
}
