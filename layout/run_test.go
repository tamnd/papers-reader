package layout

import (
	"strings"
	"testing"
)

func TestAMissingToolNamesItselfAndHowToGetIt(t *testing.T) {
	for _, tool := range Tools {
		err := (&Missing{Tool: tool}).Error()
		if !strings.Contains(err, string(tool)) {
			t.Errorf("%q does not name the tool: %s", tool, err)
		}
		if !strings.Contains(err, "pip install") {
			t.Errorf("%q does not say how to get it: %s", tool, err)
		}
	}
}

func TestEachToolHasACommandLine(t *testing.T) {
	for _, tool := range Tools {
		name, args, err := commandLine(tool, "pdf/invented.pdf", "work/invented/layout")
		if err != nil {
			t.Errorf("%q: %v", tool, err)
			continue
		}
		if name == "" {
			t.Errorf("%q has no program to run", tool)
		}
		line := name + " " + strings.Join(args, " ")
		if !strings.Contains(line, "pdf/invented.pdf") {
			t.Errorf("%q is not given the PDF: %s", tool, line)
		}
		if !strings.Contains(line, "work/invented/layout") {
			t.Errorf("%q is not given the output directory: %s", tool, line)
		}
	}
}

func TestATheCommandLineRefusesAToolItDoesNotRun(t *testing.T) {
	if _, _, err := commandLine("olmocr", "a.pdf", "out"); err == nil {
		t.Fatal("a tool this does not run was accepted")
	}
}

func TestTheCommandIsTheOneTheProgramInstallsAs(t *testing.T) {
	// marker installs as marker_single and the package is called marker-pdf,
	// which is two chances to look for the wrong program on the path.
	if got, want := command(Marker), "marker_single"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := command("olmocr"); got != "" {
		t.Errorf("got %q, want nothing", got)
	}
}

func TestNothingInstalledIsAClearError(t *testing.T) {
	// Whatever is on this machine, the error for an empty list has to say
	// what to install rather than fail with an exec error later.
	if len(Installed()) > 0 {
		t.Skip("a layout tool is installed on this machine")
	}
	_, err := Pick()
	if err == nil {
		t.Fatal("nothing is installed and Pick found something")
	}
	if !strings.Contains(err.Error(), "mineru") {
		t.Errorf("the error is %q, want the tools named in it", err)
	}
}

func TestTheLastLineIsWhatWentWrong(t *testing.T) {
	out := "Traceback (most recent call last):\n  File \"x.py\", line 1\nValueError: no pages\n\n"
	if got, want := last(out), "ValueError: no pages"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := last("   \n\n"); got != "" {
		t.Errorf("got %q, want nothing", got)
	}
}
