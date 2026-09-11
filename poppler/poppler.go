// Package poppler runs the poppler command line tools and parses what they
// print.
//
// Poppler is not a dependency of this module and never will be. It is four
// programs that have been on every Linux box and in every package manager for
// twenty years, they read a PDF far better than anything that could be
// written here, and shelling out to them costs one process per page band. A
// pure Go PDF parser would be a year of work to get to the same place and
// would be wrong about a different set of files.
//
// So the rule is: the toolchain asks poppler for facts about a file and makes
// its own decisions. `papers doctor` reports which of the programs are
// present, and anything that needs one says so by name when it is missing
// rather than failing with an exec error.
package poppler

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Tools is every program this package runs, in the order a person would
// install them. pdftoppm is not used until the vision path arrives in M3 and
// is listed so that doctor can report on it before it is needed rather than
// at the point somebody is waiting on a rebuild.
var Tools = []string{"pdfinfo", "pdftotext", "pdffonts", "pdfimages", "pdftoppm"}

// Missing is the error for a program that is not installed. It names the
// program and says what it is for, because "exec: pdffonts: not found" is
// true and unhelpful.
type Missing struct {
	Tool string
	For  string
}

func (m *Missing) Error() string {
	return fmt.Sprintf("%s is not installed, and it is what %s: install poppler (poppler-utils on Debian, poppler on Homebrew)", m.Tool, m.For)
}

// Have reports whether a program is on the path.
func Have(tool string) bool {
	_, err := exec.LookPath(tool)
	return err == nil
}

// Version is what a program says about itself, for doctor to print. Poppler
// writes its version banner to stderr and exits non-zero for -v, which is
// why this looks at both streams and ignores the status.
func Version(ctx context.Context, tool string) (string, error) {
	if !Have(tool) {
		return "", &Missing{Tool: tool, For: "reports its own version"}
	}
	cmd := exec.CommandContext(ctx, tool, "-v")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	_ = cmd.Run()
	for _, line := range strings.Split(out.String()+"\n"+errb.String(), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line, nil
		}
	}
	return "", fmt.Errorf("%s said nothing about its version", tool)
}

// run is every call in this package: bounded, with stderr kept for the error
// message, because poppler explains itself there and throwing that away
// turns a clear complaint about an encrypted file into "exit status 1".
func run(ctx context.Context, tool, purpose string, args ...string) ([]byte, error) {
	if !Have(tool) {
		return nil, &Missing{Tool: tool, For: purpose}
	}
	cmd := exec.CommandContext(ctx, tool, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errb.String()); msg != "" {
			return nil, fmt.Errorf("%s: %s", tool, firstLine(msg))
		}
		return nil, fmt.Errorf("%s: %w", tool, err)
	}
	return out.Bytes(), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Doc is what pdfinfo knows about a file.
type Doc struct {
	Pages     int
	Width     float64
	Height    float64
	Encrypted bool
	Producer  string
	Title     string
}

// Info reads the document header.
func Info(ctx context.Context, path string) (Doc, error) {
	out, err := run(ctx, "pdfinfo", "reads how many pages a PDF has", path)
	if err != nil {
		return Doc{}, err
	}
	d := ParseInfo(out)
	if d.Pages == 0 {
		return d, fmt.Errorf("pdfinfo found no pages in %s, which means it is not a PDF this can read", path)
	}
	return d, nil
}

// ParseInfo reads what pdfinfo prints.
func ParseInfo(out []byte) Doc {
	var d Doc
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "Pages":
			d.Pages, _ = strconv.Atoi(value)
		case "Encrypted":
			d.Encrypted = !strings.HasPrefix(value, "no")
		case "Producer":
			d.Producer = value
		case "Title":
			d.Title = value
		case "Page size":
			// "612 x 792 pts (letter)"
			fields := strings.Fields(value)
			if len(fields) >= 3 {
				d.Width, _ = strconv.ParseFloat(fields[0], 64)
				d.Height, _ = strconv.ParseFloat(fields[2], 64)
			}
		}
	}
	return d
}

// Page reads the text of a range of pages, keeping the layout, which is what
// tells a two column paper from a one column one later.
func Page(ctx context.Context, path string, first, last int) (string, error) {
	out, err := run(ctx, "pdftotext", "reads the text layer of a PDF",
		"-layout", "-f", strconv.Itoa(first), "-l", strconv.Itoa(last), path, "-")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Font is one font used in a document.
type Font struct {
	Name     string
	Type     string
	Embedded bool
	Unicode  bool
}

// FontList is what pdffonts reports over a range of pages.
//
// The interesting column is the last one, uni, which says whether the font
// carries a ToUnicode map. A font without one is a font whose glyphs cannot
// be turned back into characters reliably, and a page full of those is a page
// whose text layer is decoration.
func FontList(ctx context.Context, path string, first, last int) ([]Font, error) {
	out, err := run(ctx, "pdffonts", "reads which fonts a PDF uses",
		"-f", strconv.Itoa(first), "-l", strconv.Itoa(last), path)
	if err != nil {
		return nil, err
	}
	return ParseFonts(out), nil
}

// ParseFonts reads what pdffonts prints.
func ParseFonts(out []byte) []Font {
	var fonts []Font
	for i, line := range strings.Split(string(out), "\n") {
		if i < 2 || strings.TrimSpace(line) == "" {
			continue // the header and its underline
		}
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		// name type encoding emb sub uni object ID
		fonts = append(fonts, Font{
			Name:     f[0],
			Type:     strings.Join(f[1:len(f)-6+1], " "),
			Embedded: f[len(f)-5] == "yes",
			Unicode:  f[len(f)-3] == "yes",
		})
	}
	return fonts
}

// Image is one image on one page.
type Image struct {
	Page   int
	Width  int
	Height int
	Colour string
	Kind   string
	// XPPI and YPPI are how densely the image is placed on the page. They are
	// the only way to work out how much of the page it covers, because the
	// pixel size on its own says nothing: 2550 pixels is a full page at 300
	// dots per inch and a postage stamp at 3000.
	XPPI int
	YPPI int
}

// ImageList is what pdfimages reports over a range of pages.
//
// This is how a scan is told from a born digital file. A scanned page is one
// image the size of the page, and a born digital page with a photograph in it
// is a small image inside a lot of text.
func ImageList(ctx context.Context, path string, first, last int) ([]Image, error) {
	out, err := run(ctx, "pdfimages", "reads which images a PDF holds",
		"-list", "-f", strconv.Itoa(first), "-l", strconv.Itoa(last), path)
	if err != nil {
		return nil, err
	}
	return ParseImages(out), nil
}

// ParseImages reads what pdfimages -list prints.
func ParseImages(out []byte) []Image {
	var images []Image
	for i, line := range strings.Split(string(out), "\n") {
		if i < 2 || strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		// page num type width height color ...
		img := Image{Kind: f[2], Colour: f[5]}
		img.Page, _ = strconv.Atoi(f[0])
		img.Width, _ = strconv.Atoi(f[3])
		img.Height, _ = strconv.Atoi(f[4])
		if len(f) > 13 {
			img.XPPI, _ = strconv.Atoi(f[12])
			img.YPPI, _ = strconv.Atoi(f[13])
		}
		images = append(images, img)
	}
	return images
}
