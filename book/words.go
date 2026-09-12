package book

import "github.com/tamnd/papers-reader/corpus"

// Wordlist is the handful of words a set document prints that the paper did
// not: the heading over the abstract, the word in front of a figure number,
// the title of the contents page.
//
// They are here rather than in the glossary because they are not the paper's
// vocabulary, they are the furniture of the document, and because they are
// needed before any model has been asked anything. LaTeX ships them for
// English and for the languages babel covers, and it covers none of these
// three, so the four words are written down.
//
// The figure word is the one exception and it is not read from here when the
// corpus knows better. A paper that prints its captions as "Hinh 1" says so
// in every caption, and Book.FigureName reads it off the text rather than
// letting this table contradict it.
type Wordlist struct {
	Abstract   string
	Contents   string
	References string
	Figure     string
	Table      string
	Colophon   string
}

var wordlists = map[corpus.Lang]Wordlist{
	corpus.EN: {"Abstract", "Contents", "References", "Figure", "Table", "How this was made"},
	corpus.VI: {"Tóm tắt", "Mục lục", "Tài liệu tham khảo", "Hình", "Bảng", "Tài liệu này được tạo ra như thế nào"},
	corpus.ZH: {"摘要", "目录", "参考文献", "图", "表", "本文档的制作方式"},
	corpus.JA: {"要旨", "目次", "参考文献", "図", "表", "この文書の作られ方"},
}

// Words is the wordlist for a language, and English for anything else, which
// is a book set in a language the corpus does not publish and cannot happen
// from the command line.
func Words(l corpus.Lang) Wordlist {
	if w, ok := wordlists[l]; ok {
		return w
	}
	return wordlists[corpus.EN]
}
