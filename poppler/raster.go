package poppler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A Raster is how a page is turned into a picture: the resolution, and
// whether the colour is kept.
//
// Grey is the default everywhere in this toolchain because almost every page
// of almost every paper is black ink on white paper, and a colour render of
// such a page is three times the bytes for the same words. Colour is asked
// for per page, where the page really has a colour image on it.
type Raster struct {
	DPI  int
	Gray bool
}

// ToPNG renders one page of a PDF to out.
//
// One process per page. pdftoppm will happily do a range in one run, but then
// it names the files itself: the prefix, a dash, and the page number padded to
// the width of the last page of the whole document, which is a width this
// would have to ask pdfinfo for and then hope poppler agrees about. With -f N
// -l N -singlefile the name is the prefix and .png and there is nothing to
// guess. A page is also the unit of work everywhere else here, so one process
// per page is the shape the rest of the toolchain wants anyway.
//
// The render goes to a temporary directory and is renamed into place, because
// a run that is killed halfway through writing a PNG leaves a file that every
// later run sees, believes and never renders again.
func ToPNG(ctx context.Context, path string, page int, r Raster, out string) error {
	if !strings.HasSuffix(out, ".png") {
		return fmt.Errorf("%s: a rendered page is written as a .png", out)
	}
	if r.DPI <= 0 {
		return fmt.Errorf("%d is not a resolution to render at", r.DPI)
	}
	if page <= 0 {
		return fmt.Errorf("%d is not a page", page)
	}
	dir := filepath.Dir(out)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(dir, ".render-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	prefix := filepath.Join(tmp, "page")
	args := []string{"-png", "-r", strconv.Itoa(r.DPI)}
	if r.Gray {
		args = append(args, "-gray")
	}
	args = append(args,
		"-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-singlefile",
		path, prefix)
	if _, err := run(ctx, "pdftoppm", "rasterises a page for a model to read", args...); err != nil {
		return err
	}
	made := prefix + ".png"
	info, err := os.Stat(made)
	if err != nil {
		return fmt.Errorf("pdftoppm wrote nothing for page %d of %s", page, path)
	}
	if info.Size() == 0 {
		return fmt.Errorf("pdftoppm wrote an empty file for page %d of %s", page, path)
	}
	return os.Rename(made, out)
}
