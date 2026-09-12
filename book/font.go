package book

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/tamnd/papers-reader/corpus"
)

// No TeX distribution ships a Chinese or a Japanese face. They are tens of
// megabytes each and there are several of them, so tectonic's bundle has none
// and a book in those two languages has to borrow one off the machine it is
// built on.
//
// These are the families to look for, best first. The first three groups are
// what a Mac has out of the box and the rest are what a Linux box has once
// anybody has installed a CJK font at all, which on a workstation is usually
// Noto. A serif face is preferred to a sans one because a paper is a paper.
var cjkFonts = map[corpus.Lang][]string{
	corpus.ZH: {
		"Songti SC", "STSong", "Hiragino Sans GB", "PingFang SC",
		"Noto Serif CJK SC", "Source Han Serif SC", "Noto Sans CJK SC", "WenQuanYi Zen Hei",
	},
	corpus.JA: {
		"Hiragino Mincho ProN", "Hiragino Sans", "YuMincho", "Yu Mincho",
		"Noto Serif CJK JP", "Source Han Serif JP", "Noto Sans CJK JP", "IPAexMincho",
	},
}

// CJKFont finds a face the Chinese or the Japanese can be set in, and the
// empty string when the machine has none.
//
// It asks fontconfig, which is the only way to know a family name is the one
// XeTeX will resolve. Matching is not enough by itself: fc-match answers with
// the default face rather than nothing when a family is absent, so the answer
// is compared against what was asked for.
//
// An empty answer is not an error here. The caller turns it into one, with the
// name of the flag to pass, because a person who knows which font they want
// should not have to install fontconfig to say so.
func CJKFont(l corpus.Lang) string {
	for _, name := range cjkFonts[l] {
		if haveFont(name) {
			return name
		}
	}
	return ""
}

// haveFont says fontconfig knows a family by that name.
func haveFont(name string) bool {
	out, err := exec.Command("fc-list", "--format", "%{family}\n", ":family="+name).Output()
	if err != nil {
		return macFont(name)
	}
	for _, line := range strings.Split(string(out), "\n") {
		for _, family := range strings.Split(line, ",") {
			if strings.EqualFold(strings.TrimSpace(family), name) {
				return true
			}
		}
	}
	return false
}

// macFont is the fallback for a Mac with no fontconfig, which is a Mac where
// nobody has installed anything from Homebrew. XeTeX on a Mac resolves a
// family through Core Text and does not need fontconfig at all, so refusing
// to build there would be refusing on the evidence of a missing tool rather
// than of a missing font.
func macFont(name string) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	out, err := exec.Command("system_profiler", "-json", "SPFontsDataType").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), `"`+name+`"`)
}
