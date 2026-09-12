package refs

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// fields fills in everything about an entry that can be read off its raw
// text. Nothing here can fail: a field that cannot be read is left empty,
// and an entry with nothing but raw is a perfectly good entry.
func fields(s Style, e *Entry) {
	raw := e.Raw
	e.DOI = find(doiPattern, raw)
	e.ArXiv = arxivID(raw)
	e.URL = find(urlPattern, raw)
	e.Pages = pageRange(raw)
	if s == StyleAuthorYear {
		authorYearFields(e)
	} else {
		listFields(e)
	}
	if e.Year == 0 {
		e.Year = year(raw)
	}
}

var (
	doiPattern   = regexp.MustCompile(`\b10\.\d{4,9}/[^\s,;"')\]]+`)
	arxivPattern = regexp.MustCompile(`(?i)arxiv[:/ ]\s*((?:\d{4}\.\d{4,5}|[a-z-]+(?:\.[A-Z]{2})?/\d{7})(?:v\d+)?)`)
	absPattern   = regexp.MustCompile(`\babs/((?:\d{4}\.\d{4,5})(?:v\d+)?)`)
	urlPattern   = regexp.MustCompile(`https?://[^\s,;"')\]]+`)
	yearPattern  = regexp.MustCompile(`\b((?:1[6-9]|20)\d{2})\b`)
	pagesPattern = regexp.MustCompile(`(?i)\b(?:pages|pp?)\.?\s*(\d+)\s*[-–—]+\s*(\d+)`)
)

func find(re *regexp.Regexp, s string) string {
	m := re.FindString(s)
	return strings.TrimRight(m, ".,;:")
}

// arxivID reads the arXiv identifier, which a reference writes as
// "arXiv:1607.06450" when it is citing the preprint and as "CoRR,
// abs/1409.0473" when it is citing the report series. Both are the same
// paper and both resolve.
func arxivID(s string) string {
	if m := arxivPattern.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	if m := absPattern.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// year is the publication year, taken as the last four digit year in the
// entry rather than the first.
//
// The last one because an entry that names two years names the volume year
// first and the publication year last, as in "Bell System Technical Journal,
// 27, 1948". A reprint gives the wrong answer either way and there is no
// reading of the text that fixes that.
func year(s string) int {
	m := yearPattern.FindAllStringSubmatch(s, -1)
	if len(m) == 0 {
		return 0
	}
	n, _ := strconv.Atoi(m[len(m)-1][1])
	return n
}

// pageRange is the page span. It is read from the entry saying so where the
// entry says so, because a bare pair of numbers with a dash between them is
// as likely to be a year range or part of a report number, and a wrong page
// range in a citation is the kind of mistake nobody catches by reading.
//
// The older journals say so by position instead of in words. A 1970
// Communications of the ACM entry ends "Comm. ACM 12, 9 (Sept. 1969),
// 501-507." and there is no "pp." anywhere in it, which is the style half the
// bibliographies in this corpus are set in. So a bare range is believed in the
// one place it is unambiguous: at the very end of the entry, after a comma,
// and not reading as a span of years.
func pageRange(s string) string {
	if m := pagesPattern.FindStringSubmatch(s); m != nil {
		return m[1] + "-" + m[2]
	}
	m := trailingPages.FindStringSubmatch(s)
	if m == nil || looksLikeYears(m[1], m[2]) {
		return ""
	}
	return m[1] + "-" + m[2]
}

// trailingPages is a page span written as the last thing in the entry with
// nothing in front of it to say that is what it is.
var trailingPages = regexp.MustCompile(`,\s*(\d{1,4})\s*[-–—]+\s*(\d{1,4})\.?\s*$`)

// looksLikeYears says whether a bare range is a span of years rather than of
// pages, as in a journal that ran from 1968 to 1972. Both ends have to read as
// a year: a paper really can start on page 1948, and one that does is at the
// end of a volume whose other end is a four figure number too.
func looksLikeYears(from, to string) bool {
	return isYear(from) && isYear(to)
}

func isYear(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && len(s) == 4 && n >= 1600 && n <= 2099
}

// listFields parses an entry from a numbered or bracketed bibliography,
// where the label has already been taken off and what is left is the
// author list, the title and the venue, in that order, separated by full
// stops or by the quotation marks around the title.
func listFields(e *Entry) {
	raw := e.Raw
	if title, before, after, ok := quotedTitle(raw); ok {
		e.Title = title
		e.Authors = names(before)
		e.Venue = venue(after)
		return
	}
	parts := sentences(raw)
	switch len(parts) {
	case 0:
		return
	case 1:
		e.Title = clean(parts[0])
		return
	}
	if looksLikeAuthors(parts[0]) {
		authors, title := cutTrailingTitle(parts[0])
		e.Authors = names(authors)
		if title != "" {
			e.Title = clean(title)
			e.Venue = venue(strings.Join(parts[1:], ". "))
			return
		}
		e.Title = clean(parts[1])
		e.Venue = venue(strings.Join(parts[2:], ". "))
		return
	}
	e.Title = clean(parts[0])
	e.Venue = venue(strings.Join(parts[1:], ". "))
}

// authorYearFields parses "Shannon, C. E. (1948). A mathematical theory of
// communication. Bell System Technical Journal, 27, 379-423."
//
// This style is the easy one: the year in brackets marks where the authors
// end, so the author list does not have to be guessed at.
func authorYearFields(e *Entry) {
	m := yearLabel.FindStringSubmatchIndex(e.Raw)
	if m == nil || m[0] != 0 {
		listFields(e)
		return
	}
	e.Authors = names(e.Raw[m[2]:m[3]])
	e.Year = year(e.Raw[m[4]:m[5]])
	parts := sentences(e.Raw[m[1]:])
	if len(parts) > 0 {
		e.Title = clean(parts[0])
		e.Venue = venue(strings.Join(parts[1:], ". "))
	}
}

// quotedTitle pulls out a title the typesetter marked with quotation marks,
// which is the strongest signal a reference gives.
func quotedTitle(raw string) (title, before, after string, ok bool) {
	for _, pair := range [][2]string{{`“`, `”`}, {`"`, `"`}, {`‘`, `’`}} {
		i := strings.Index(raw, pair[0])
		if i < 0 {
			continue
		}
		j := strings.Index(raw[i+len(pair[0]):], pair[1])
		if j < 0 {
			continue
		}
		j += i + len(pair[0])
		title = clean(raw[i+len(pair[0]) : j])
		if title == "" {
			continue
		}
		return title, raw[:i], raw[j+len(pair[1]):], true
	}
	return "", "", "", false
}

// sentences cuts a reference at the full stops that end a field, which is
// not every full stop in it.
//
// An initial is the common one: "Quoc V. Le" has a full stop in the middle
// of a name, and cutting there would file half the authors as the title.
// The abbreviations journals use in a venue are the other one.
func sentences(s string) []string {
	var out []string
	r := []rune(s)
	start := 0
	for i := 0; i < len(r); i++ {
		if r[i] != '.' || i+1 >= len(r) || r[i+1] != ' ' {
			continue
		}
		if abbreviated(string(r[start:i])) {
			continue
		}
		if part := strings.TrimSpace(string(r[start:i])); part != "" {
			out = append(out, part)
		}
		start = i + 1
	}
	if part := strings.TrimSpace(string(r[start:])); part != "" {
		out = append(out, part)
	}
	return out
}

// abbreviated says whether the full stop that follows this text is part of
// an abbreviation. A word with a full stop already inside it is one, which
// covers i.e., e.g. and U.S. without listing them.
func abbreviated(before string) bool {
	word := lastWord(before)
	if word == "" {
		return false
	}
	if strings.Contains(word, ".") {
		return true
	}
	if r := []rune(word); len(r) == 1 && unicode.IsUpper(r[0]) {
		return true
	}
	return abbreviations[strings.ToLower(word)]
}

// abbreviations is the short words a reference ends with a full stop and
// keeps going. It is the set that actually turns up in a bibliography and
// not a general list: the cost of a missing one is a title that keeps its
// venue, and the cost of a wrong one is an entry that never gets cut.
var abbreviations = map[string]bool{
	"al": true, "ed": true, "eds": true, "edn": true, "rev": true,
	"vol": true, "vols": true, "no": true, "nos": true, "pp": true, "p": true,
	"jr": true, "sr": true, "dr": true, "prof": true, "mr": true, "ms": true,
	"inc": true, "ltd": true, "co": true, "univ": true, "dept": true,
	"proc": true, "conf": true, "symp": true, "int": true, "intl": true,
	"trans": true, "j": true, "sci": true, "ann": true, "tech": true,
	"rep": true, "ch": true, "chap": true, "fig": true, "figs": true,
	"sec": true, "sect": true, "cf": true, "etc": true, "st": true,
	"assoc": true, "soc": true, "res": true, "eng": true, "comput": true,
	"syst": true, "appl": true, "math": true, "phys": true, "amer": true,
}

func lastWord(s string) string {
	s = strings.TrimRight(s, " \t")
	if i := strings.LastIndexAny(s, " \t"); i >= 0 {
		s = s[i+1:]
	}
	return strings.Trim(s, "([{«\"'“‘")
}

// looksLikeAuthors says whether the first field of an entry is the author
// list or whether the entry begins with its title, which happens for a
// standard, a manual or a report with no named author.
func looksLikeAuthors(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len([]rune(s)) > 200 {
		return false
	}
	// A colon is a subtitle, and a subtitle means a title.
	if strings.ContainsAny(s, ":?") {
		return false
	}
	if r := []rune(s); !unicode.IsUpper(r[0]) {
		return false
	}
	if strings.Contains(s, ",") || conjunction.MatchString(s) {
		return true
	}
	// One author, written out: two or three capitalised words, no more.
	words := strings.Fields(s)
	if len(words) < 2 || len(words) > 4 {
		return false
	}
	for _, w := range words {
		if r := []rune(w); !unicode.IsUpper(r[0]) {
			return false
		}
	}
	return true
}

// cutTrailingTitle rescues the one author list that cannot be cut off by a
// full stop: the surname first list whose last author ends in an initial.
//
// "Nkemelu, A. B. and Oyelaran, C. A theory of slow indexes." has its only
// sentence ending full stop after the "C", which is an initial, so the
// title stays in the author field and the whole entry parses wrong.
//
// The tell is length. What follows the last initial is either a surname, of
// one or two words, or it is a title, of four or more. It is only looked
// for in a list that is written surname first, which is a list whose first
// field is a bare surname with no initial in it, because in a list written
// the other way round the first initial of the first author would be read
// as the end of the names.
func cutTrailingTitle(chunk string) (authors, title string) {
	parts := strings.Split(chunk, ",")
	if len(parts) < 2 || !bareSurname(parts[0]) {
		return chunk, ""
	}
	last := parts[len(parts)-1]
	m := trailingInitials.FindStringSubmatch(last)
	if m == nil || len(strings.Fields(m[2])) < 4 {
		return chunk, ""
	}
	parts[len(parts)-1] = m[1]
	return strings.Join(parts, ","), m[2]
}

var trailingInitials = regexp.MustCompile(`^\s*((?:\p{Lu}\.\s*)+)(\p{Lu}\S*(?:\s+\S+)*)$`)

// bareSurname says whether a field is a surname on its own, with no initial
// in it, which is how a surname first author list starts.
func bareSurname(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || strings.Contains(s, ".") || len(strings.Fields(s)) > 3 {
		return false
	}
	return unicode.IsUpper([]rune(s)[0])
}

// conjunction is what joins the last two authors of a list. Case insensitive
// because the journals that set author names in small capitals set the word
// between them in small capitals too, and an extractor reads small capitals
// as capitals: "BRIGHTWELL, M. T., AND DUNNE, R. Q." is two authors, and read
// case sensitively the second of them is called "R. Q. AND DUNNE".
//
// The spaces are required on both sides, so a surname that ends in those three
// letters and a corporation called Rand are left alone.
var conjunction = regexp.MustCompile(`(?i)\s+and\s+|\s*&\s*`)

// names splits an author list into one name per author.
//
// Both orders turn up and they have to be told apart: "Jimmy Lei Ba, Jamie
// Ryan Kiros" is two authors given first, and "Shannon, C. E." is one author
// surname first. The tell is a field that is nothing but initials, which is
// never a name on its own, so it belongs to the field before it.
func names(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "In ")
	s = strings.Trim(s, " ,;")
	if s == "" {
		return nil
	}
	s = conjunction.ReplaceAllString(s, ", ")
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" || isEtAl(part) {
			continue
		}
		// The full stop of an initial is part of the name and the one at the
		// end of the author list is punctuation. They are the same character
		// and only the shape of the field tells them apart.
		if initialsOnly(part) {
			if len(out) > 0 {
				out[len(out)-1] = part + " " + out[len(out)-1]
				continue
			}
		}
		out = append(out, strings.TrimRight(part, "."))
	}
	return out
}

func isEtAl(s string) bool {
	s = strings.ToLower(strings.Trim(s, " ."))
	return s == "et al" || s == "et al." || s == "others"
}

// initialsOnly says whether a field is nothing but initials, as in "C. E.".
func initialsOnly(s string) bool {
	fields := strings.Fields(s)
	if len(fields) == 0 || len(fields) > 4 {
		return false
	}
	for _, f := range fields {
		f = strings.TrimRight(f, ".")
		if len([]rune(f)) != 1 {
			return false
		}
		if r := []rune(f)[0]; !unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

// venue tidies what is left over after the authors and the title. It is not
// parsed any further: the venue is never used for resolution and the page
// renders raw, so a tidy string is all it has to be.
//
// What it does drop is the URL and the bare year, because those are already
// fields of their own and a venue of "1957" tells a reader nothing they
// cannot see two lines further down.
func venue(s string) string {
	s = urlPattern.ReplaceAllString(s, "")
	s = clean(datePattern.ReplaceAllString(clean(s), ""))
	if len([]rune(s)) > 160 || !hasLetter(s) {
		return ""
	}
	return s
}

// datePattern is the date of publication where a venue ends in one, which
// is almost everywhere. It comes off because the year is a field of its own
// and a venue reading "Journal of Something, April 1991" says the same
// thing twice.
var datePattern = regexp.MustCompile(`(?i)[,\s]*(?:january|february|march|april|may|june|july|august|september|october|november|december)?[,\s]*\b(?:1[6-9]|20)\d{2}[a-z]?\.?\s*$`)

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// flatten collapses the whitespace a two column extraction leaves behind
// and does nothing else. It is what raw goes through, and raw has to stay
// the reference as the paper printed it.
func flatten(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// clean is flatten plus the punctuation that belonged to the separator
// rather than to the field, which is right for a parsed field and wrong for
// raw.
func clean(s string) string {
	return strings.Trim(flatten(s), " ,;.")
}
