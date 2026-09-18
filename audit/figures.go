package audit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
	"github.com/tamnd/papers-reader/figures"
)

// figuresRules is group F, the rules over the one part of the corpus that is
// not text.
//
// Eight of the nine are hard, which is more than any other group, and that
// is deliberate. A figure is a binary in a public repository, so a mistake
// here is not a paragraph somebody has to reread: it is a piece of a
// copyrighted paper republished, or a file that does not open, or a picture
// with nothing anywhere to say what it is. F06 is the one that matters most
// and it is the reason the whole group exists.
func figuresRules() []Rule {
	return []Rule{
		{
			ID: "F01", Hard: true,
			What:  "every figure a file references exists on disk.",
			Check: ruleF01,
		},
		{
			ID: "F02", Hard: true,
			What:  "no figure is under 100 by 100 pixels.",
			Check: ruleF02,
		},
		{
			ID: "F03", Hard: true,
			What:  "no figure is over 512 KB.",
			Check: ruleF03,
		},
		{
			ID: "F04", Hard: true,
			What:  "nothing under figures/ is untracked.",
			Check: ruleF04,
		},
		{
			ID: "F05", Hard: true,
			What:  "no paper has two figures with the same bytes.",
			Check: ruleF05,
		},
		{
			ID: "F06", Hard: true,
			What:  "no figure covers more than 0.75 of the page it came from.",
			Check: ruleF06,
		},
		{
			ID: "F07", Hard: true,
			What:  "every committed figure has an entry in manifests/figures.yaml with a caption.",
			Check: ruleF07,
		},
		{
			ID: "F08", Hard: true,
			What:  "no restricted paper has a figure.",
			Check: ruleF08,
		},
		{
			ID:    "F09",
			What:  "every figure the paper numbers in its prose is present.",
			Check: ruleF09,
		},
	}
}

// onDisk is every committed figure, as paths relative to the corpus root,
// keyed by paper. It is the disk and not the manifest, because half of this
// group is about the two disagreeing.
func onDisk(in *Input) (map[string][]string, error) {
	out := map[string][]string{}
	for _, p := range in.Papers.Papers {
		files, err := figureFiles(in, p.ID)
		if err != nil {
			return nil, err
		}
		if len(files) > 0 {
			out[p.ID] = files
		}
	}
	return out, nil
}

// anyFigures says whether this group has anything to look at. A corpus
// partway through extraction has papers with no figures yet, and a rule that
// passed on them would be claiming to have checked something.
func anyFigures(in *Input) (map[string][]string, bool, error) {
	files, err := onDisk(in)
	if err != nil {
		return nil, false, err
	}
	return files, len(files) > 0 || len(in.Figures.Figures) > 0, nil
}

var imageLink = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)`)

// ruleF01 follows the references rather than the files. A figure nobody
// points at is dead weight; a pointer to a figure that is not there is a
// broken image in the reading app, and the reader sees it before anybody
// running the audit does.
func ruleF01(in *Input) ([]Finding, error) {
	_, any, err := anyFigures(in)
	if err != nil {
		return nil, err
	}
	if !any {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, p := range in.Papers.Papers {
		// The manifest is a reference too, and the commonest way to break
		// this rule is to commit the manifest and forget the PNG.
		for _, f := range in.Figures.Of(p.ID) {
			path := filepath.Join(in.Corpus.Figures(p.ID), f.Name())
			if _, err := os.Stat(path); err != nil {
				out = append(out, Finding{
					Rule: "F01", File: "manifests/figures.yaml",
					Message: fmt.Sprintf("%s of %s is recorded and is not on disk", f.ID, p.ID),
				})
			}
		}
		files, err := contentFiles(in, p.ID)
		if err != nil {
			return nil, err
		}
		for _, rel := range files {
			found, err := brokenLinks(in.Corpus.Root, rel)
			if err != nil {
				return nil, err
			}
			out = append(out, found...)
		}
	}
	return out, nil
}

// brokenLinks reads one Markdown file and reports the images it points at
// that are not there. A link that leaves the corpus, to an http URL or to
// anything above the root, is somebody else's problem and is left alone.
func brokenLinks(root, rel string) ([]Finding, error) {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	var out []Finding
	for n, line := range strings.Split(string(b), "\n") {
		for _, m := range imageLink.FindAllStringSubmatch(line, -1) {
			target := m[1]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "data:") {
				continue
			}
			path := filepath.Join(root, filepath.Dir(filepath.FromSlash(rel)), filepath.FromSlash(target))
			if _, err := os.Stat(path); err == nil {
				continue
			}
			out = append(out, Finding{
				Rule: "F01", File: rel, Line: n + 1,
				Message: fmt.Sprintf("the image %s is not there", target),
			})
		}
	}
	return out, nil
}

// ruleF02 measures the PNG rather than believing the manifest, because the
// number in the manifest was written by the same run that wrote the file and
// the two agreeing proves nothing.
func ruleF02(in *Input) ([]Finding, error) {
	return eachFigure(in, "F02", func(path string, b []byte) string {
		cfg, err := png.DecodeConfig(bytes.NewReader(b))
		if err != nil {
			return fmt.Sprintf("it is not a PNG this can read: %v", err)
		}
		if err := figures.Default().Check(figures.Figure{Width: cfg.Width, Height: cfg.Height}); err != nil {
			return err.Error()
		}
		return ""
	})
}

func ruleF03(in *Input) ([]Finding, error) {
	return eachFigure(in, "F03", func(path string, b []byte) string {
		if len(b) > figures.MaxBytes {
			return fmt.Sprintf("it is %d KB and the cap is %d KB", len(b)>>10, figures.MaxBytes>>10)
		}
		return ""
	})
}

// eachFigure runs one test over every committed figure. The file is read
// once and handed to the test, because every rule in this group that looks
// at the bytes wants all of them.
func eachFigure(in *Input, rule string, test func(path string, b []byte) string) ([]Finding, error) {
	files, any, err := anyFigures(in)
	if err != nil {
		return nil, err
	}
	if !any {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, id := range ids(in) {
		for _, rel := range files[id] {
			b, err := os.ReadFile(filepath.Join(in.Corpus.Root, filepath.FromSlash(rel)))
			if err != nil {
				return nil, err
			}
			if msg := test(rel, b); msg != "" {
				out = append(out, Finding{Rule: rule, File: rel, Message: msg})
			}
		}
	}
	return out, nil
}

// ids is the papers in manifest order, so that findings come out in the same
// order twice running.
func ids(in *Input) []string {
	out := make([]string, 0, len(in.Papers.Papers))
	for _, p := range in.Papers.Papers {
		out = append(out, p.ID)
	}
	return out
}

// ruleF04 asks git rather than the disk. A figure that was rendered and
// never added is a figure the site will not have, and the person who
// rendered it is the last person who will notice.
func ruleF04(in *Input) ([]Finding, error) {
	files, any, err := anyFigures(in)
	if err != nil {
		return nil, err
	}
	if !any || in.Tracked == nil {
		return nil, ErrNotRun
	}
	tracked := make(map[string]bool, len(in.Tracked))
	for _, p := range in.Tracked {
		tracked[p] = true
	}
	var out []Finding
	for _, id := range ids(in) {
		for _, rel := range files[id] {
			if !tracked[rel] {
				out = append(out, Finding{
					Rule: "F04", File: rel,
					Message: "it is on disk and git is not holding it",
				})
			}
		}
	}
	return out, nil
}

// ruleF05 is about one paper and not about the corpus. The same diagram
// twice in one paper is one figure printed twice, and committing it twice
// gives a reader two names for one picture and translates its caption twice.
// The same diagram in two papers is two papers, and each of them keeps it.
func ruleF05(in *Input) ([]Finding, error) {
	files, any, err := anyFigures(in)
	if err != nil {
		return nil, err
	}
	if !any {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, id := range ids(in) {
		seen := map[string]string{}
		for _, rel := range files[id] {
			b, err := os.ReadFile(filepath.Join(in.Corpus.Root, filepath.FromSlash(rel)))
			if err != nil {
				return nil, err
			}
			sum := sha256.Sum256(b)
			key := hex.EncodeToString(sum[:])
			if first, ok := seen[key]; ok {
				out = append(out, Finding{
					Rule: "F05", File: rel,
					Message: fmt.Sprintf("it is byte for byte %s", filepath.Base(first)),
				})
				continue
			}
			seen[key] = rel
		}
	}
	return out, nil
}

// ruleF06 is the rule that keeps this a corpus and not a mirror.
//
// A cropped diagram is a figure. A page image committed because the
// extraction was hard is a scan of somebody's copyrighted paper in a public
// repository, and to git those two look identical. The fraction is the only
// thing that tells them apart, so it is recorded for every figure and it is
// checked here as well as at the point of writing.
//
// A figure with no fraction recorded fails too. The rule cannot be satisfied
// by leaving the field out.
func ruleF06(in *Input) ([]Finding, error) {
	if len(in.Figures.Figures) == 0 {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, f := range in.Figures.Figures {
		switch {
		case f.Fraction <= 0:
			out = append(out, Finding{
				Rule: "F06", File: "manifests/figures.yaml",
				Message: fmt.Sprintf("%s of %s records no page_fraction, so nothing says it is not a page image",
					f.ID, f.Paper),
			})
		case f.Fraction > figures.MaxFraction:
			out = append(out, Finding{
				Rule: "F06", File: "manifests/figures.yaml",
				Message: fmt.Sprintf("%s of %s covers %.0f%% of its page and the cap is %.0f%%",
					f.ID, f.Paper, f.Fraction*100, figures.MaxFraction*100),
			})
		}
	}
	return out, nil
}

// ruleF07 is the other half of F01: a file on disk that the manifest does
// not know about. A figure with no entry has no caption, so the reading app
// has nothing to put under it and the translators have nothing to translate.
func ruleF07(in *Input) ([]Finding, error) {
	files, any, err := anyFigures(in)
	if err != nil {
		return nil, err
	}
	if !any {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, id := range ids(in) {
		recorded := map[string]string{}
		for _, f := range in.Figures.Of(id) {
			recorded[f.Name()] = f.Caption
		}
		for _, rel := range files[id] {
			name := filepath.Base(rel)
			caption, ok := recorded[name]
			switch {
			case !ok:
				out = append(out, Finding{
					Rule: "F07", File: rel,
					Message: "it is committed and manifests/figures.yaml has no entry for it",
				})
			case strings.TrimSpace(caption) == "":
				out = append(out, Finding{
					Rule: "F07", File: rel,
					Message: "its entry in manifests/figures.yaml carries no caption",
				})
			}
		}
	}
	return out, nil
}

// ruleF08 is S02 asked from the figures side, and it is worth asking twice.
// A restricted paper is one the corpus may describe and may not reproduce,
// and a diagram is the part of a paper its publisher is most protective of.
func ruleF08(in *Input) ([]Finding, error) {
	if in.whole() {
		return nil, ErrNotRun
	}
	files, err := onDisk(in)
	if err != nil {
		return nil, err
	}
	restricted := 0
	var out []Finding
	for _, p := range in.Papers.Papers {
		if in.Sources.Access(p.ID) != corpus.AccessRestricted {
			continue
		}
		restricted++
		for _, rel := range files[p.ID] {
			out = append(out, Finding{
				Rule: "F08", File: rel,
				Message: fmt.Sprintf("%s is restricted, so it may have no figures at all", p.ID),
			})
		}
		for _, f := range in.Figures.Of(p.ID) {
			out = append(out, Finding{
				Rule: "F08", File: "manifests/figures.yaml",
				Message: fmt.Sprintf("%s is restricted and %s is recorded for it", p.ID, f.ID),
			})
		}
	}
	if restricted == 0 {
		return nil, ErrNotRun
	}
	return out, nil
}

// mention is a reference to a figure in the prose, and it has to read the
// number the same way the caption regexp in package figures wrote it. A
// paper that numbers by section calls its first figure 1.1, and a pattern
// that stopped at the full stop would ask every one of GPT-3's thirty four
// figures for a figure 1 that was never there.
var mention = regexp.MustCompile(`\b(?:Figure|Fig\.)\s+([0-9]{1,3}(?:[.\-][0-9a-zA-Z]+)*|[A-Z](?:[.\-][0-9a-zA-Z]+)+)\b`)

// legend is a caption standing at the head of its own line, which is how the
// assembler writes one and is not how a sentence refers to a figure.
//
// The part in brackets is a figure the paper drew in pieces and captioned
// piece by piece. Codd's Figure 3 is a set of relations before normalisation
// and the same set after it, and the page captions them "Fig. 3(a)." and
// "Fig. 3(b).". The bracket is outside the group, so both lines say the
// figure that is present is figure 3, which is the number the prose asks for
// when it says "the collection of relations exhibited in Figure 3(a)" and is
// what package figures records for a region under either caption.
var legend = regexp.MustCompile(`^(?:Figure|Fig\.)\s+([0-9]{1,3}(?:[.\-][0-9a-zA-Z]+)*|[A-Z](?:[.\-][0-9a-zA-Z]+)+)(?:\([0-9a-zA-Z]{1,2}\))?\s*[:.]`)

// ruleF09 is soft, and the reason is that it cannot tell the two causes
// apart. A paper whose prose mentions Figure 7 and which has six figures may
// have had its seventh dropped by the detector, or the seventh may be in an
// appendix nobody fetched, or the sentence may be citing another paper's
// figure. The rule's job is to say so and let a person look.
//
// A figure the manifest does not carry may still be in front of the reader.
// Not every figure is a picture. Appendix G of the GPT-3 paper is fifty one
// worked dataset examples, each one a box of text, and the extractor reads
// them as the tables they are and sets them in the markdown. The figure is
// there, better set than a screenshot of it would be, and asking for a PNG
// of it as well would be asking for the page to be read twice.
//
// So a caption of its own on a line of its own counts as the figure being
// present. That is the form the assembler writes, and prose does not use
// it: a sentence says "as Figure 3 shows" and never opens a line with
// "Figure 3:".
func ruleF09(in *Input) ([]Finding, error) {
	if len(in.Figures.Figures) == 0 {
		return nil, ErrNotRun
	}
	var out []Finding
	for _, p := range in.Papers.Papers {
		have := in.Figures.Of(p.ID)
		if len(have) == 0 {
			continue
		}
		numbered := map[string]bool{}
		for _, f := range have {
			numbered[f.Number] = true
		}
		files, err := contentFiles(in, p.ID)
		if err != nil {
			return nil, err
		}
		var prose []string
		for _, rel := range files {
			if !strings.HasPrefix(rel, "content/en/") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(in.Corpus.Root, filepath.FromSlash(rel)))
			if err != nil {
				return nil, err
			}
			for _, line := range strings.Split(string(b), "\n") {
				if m := legend.FindStringSubmatch(line); m != nil {
					numbered[m[1]] = true
					line = line[len(m[0]):]
				}
				prose = append(prose, line)
			}
		}
		missing := map[string]bool{}
		for _, line := range prose {
			for _, m := range mention.FindAllStringSubmatch(line, -1) {
				if !numbered[m[1]] {
					missing[m[1]] = true
				}
			}
		}
		for _, n := range byNumber(missing) {
			out = append(out, Finding{
				Rule: "F09", File: "manifests/figures.yaml",
				Message: fmt.Sprintf("%s mentions Figure %s in its prose and has no such figure", p.ID, n),
			})
		}
	}
	return out, nil
}

// byNumber is the figure numbers a paper is missing, in the order the paper
// would list them. Part by part and numerically where a part is a number, so
// that figure 10 comes after figure 9 and figure 3.2 after figure 3.1.
func byNumber(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return before(out[i], out[j]) })
	return out
}

func before(a, b string) bool {
	as, bs := parts(a), parts(b)
	for i := 0; i < len(as) && i < len(bs); i++ {
		if as[i] == bs[i] {
			continue
		}
		x, errx := strconv.Atoi(as[i])
		y, erry := strconv.Atoi(bs[i])
		if errx == nil && erry == nil {
			return x < y
		}
		return as[i] < bs[i]
	}
	return len(as) < len(bs)
}

func parts(n string) []string {
	return strings.FieldsFunc(n, func(r rune) bool { return r == '.' || r == '-' })
}
