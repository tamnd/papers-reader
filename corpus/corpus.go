// Package corpus models the papers corpus: paper ids, the twelve fields, the
// access classes that decide what may be published, the manifests that list
// what the corpus holds, and the front matter every content file carries.
//
// Nothing in this package touches the network or a model. It reads and writes
// files in a checkout of tamnd/papers and nothing else, which is why every
// other package can depend on it.
package corpus

import "fmt"

// Field is one of the twelve subject fields. They are the organisation of the
// seed list rather than a taxonomy of computer science, and a paper has
// exactly one.
type Field string

const (
	Theory       Field = "theory"
	Algorithms   Field = "algorithms"
	Languages    Field = "languages"
	Systems      Field = "systems"
	Networks     Field = "networks"
	Databases    Field = "databases"
	Architecture Field = "architecture"
	Security     Field = "security"
	AIML         Field = "ai-ml"
	Graphics     Field = "graphics"
	HCI          Field = "hci"
	Software     Field = "software"
)

// Fields is every field, in the order the catalogue lists them.
var Fields = []Field{
	Theory, Algorithms, Languages, Systems,
	Networks, Databases, Architecture, Security,
	AIML, Graphics, HCI, Software,
}

var fieldTitles = map[Field]string{
	Theory:       "Theory of computation and complexity",
	Algorithms:   "Algorithms and data structures",
	Languages:    "Programming languages, compilers, runtimes",
	Systems:      "Operating systems and distributed systems",
	Networks:     "Computer networks",
	Databases:    "Databases and data systems",
	Architecture: "Computer architecture",
	Security:     "Security and cryptography",
	AIML:         "Artificial intelligence and machine learning",
	Graphics:     "Computer graphics",
	HCI:          "Human-computer interaction",
	Software:     "Software engineering",
}

// Title is the field written out for a reader.
func (f Field) Title() string { return fieldTitles[f] }

// Valid reports whether f is one of the twelve.
func (f Field) Valid() bool { _, ok := fieldTitles[f]; return ok }

// ParseField turns the manifest spelling into a Field.
func ParseField(s string) (Field, error) {
	f := Field(s)
	if !f.Valid() {
		return "", fmt.Errorf("%q is not one of the twelve fields", s)
	}
	return f, nil
}

// Access is what the licence of a paper permits the corpus to publish. It is
// the single most consequential field in the corpus: everything downstream
// asks it before writing a file.
type Access string

const (
	AccessPublicDomain Access = "public-domain"
	AccessOpen         Access = "open"
	AccessPermissive   Access = "permissive"
	AccessRestricted   Access = "restricted"
	AccessUnknown      Access = "unknown"
)

// Accesses is every access class, most permissive first.
var Accesses = []Access{AccessPublicDomain, AccessOpen, AccessPermissive, AccessRestricted, AccessUnknown}

var accessClasses = map[Access]bool{
	AccessPublicDomain: true, AccessOpen: true, AccessPermissive: true,
	AccessRestricted: true, AccessUnknown: true,
}

// Valid reports whether a is one of the five access classes. An empty access
// is not valid, and is treated as AccessUnknown everywhere it is read, because a
// paper nobody has looked at and a paper somebody looked at and could not
// place should behave the same way.
func (a Access) Valid() bool { return accessClasses[a] }

// Body reports whether the body text of the paper may be published.
func (a Access) Body() bool {
	switch a {
	case AccessPublicDomain, AccessOpen, AccessPermissive:
		return true
	}
	return false
}

// Figures reports whether cropped figures from the paper may be committed.
func (a Access) Figures() bool { return a.Body() }

// Abstract reports whether a short quoted abstract may be published. A
// restricted paper gets one under fair dealing, capped by audit rule S07; a
// paper with no access class gets nothing at all.
func (a Access) Abstract() bool { return a.Body() || a == AccessRestricted }

// Status is how far a paper has come down the pipeline. It is a claim about
// work done, not about quality: the audit is what says whether a file that
// exists is any good.
type Status string

const (
	Listed     Status = "listed"
	Fetched    Status = "fetched"
	Extracted  Status = "extracted"
	Translated Status = "translated"
	Done       Status = "done"
)

// Statuses is every status in pipeline order.
var Statuses = []Status{Listed, Fetched, Extracted, Translated, Done}

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	for _, k := range Statuses {
		if k == s {
			return true
		}
	}
	return false
}

// Lang is a corpus language. English is the extracted text and the other
// three are translations of it.
type Lang string

const (
	EN Lang = "en"
	VI Lang = "vi"
	ZH Lang = "zh"
	JA Lang = "ja"
)

// Langs is every language, English first.
var Langs = []Lang{EN, VI, ZH, JA}

// Valid reports whether l is a corpus language.
func (l Lang) Valid() bool {
	for _, k := range Langs {
		if k == l {
			return true
		}
	}
	return false
}

// Translated reports whether l is one of the three translations.
func (l Lang) Translated() bool { return l.Valid() && l != EN }

// Name is the language in English, for a prompt and for a line on a
// terminal. An unknown code is returned as it came, because a message about
// a code nobody recognises should say which code.
func (l Lang) Name() string {
	switch l {
	case EN:
		return "English"
	case VI:
		return "Vietnamese"
	case ZH:
		return "Chinese"
	case JA:
		return "Japanese"
	}
	return string(l)
}
