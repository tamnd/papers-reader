package corpus

import (
	"os"
	"path/filepath"
	"testing"
)

// The default has to be the cautious one. A checkout with no policy file is
// every checkout that existed before the file did, and none of them agreed
// to publish anybody's restricted paper in full.
func TestACorpusWithNoPolicyFilePublishesByLicence(t *testing.T) {
	c := testCorpus(t, threePapers, "", testSources)
	if c.PublishesWhole() {
		t.Error("a corpus with no policy file says it publishes everything")
	}
	if c.Publishes(AccessRestricted) {
		t.Error("a restricted paper is published with no policy file")
	}
	if !c.Publishes(AccessOpen) {
		t.Error("an open paper is not published")
	}
}

func TestAPolicyThatPublishesBodiesCoversEveryAccessClass(t *testing.T) {
	c := policyCorpus(t, "body: true\n")
	if !c.PublishesWhole() {
		t.Fatal("the policy did not take")
	}
	for _, a := range Accesses {
		if !c.Publishes(a) {
			t.Errorf("%s is not published under a body policy", a)
		}
		if !c.PublishesFigures(a) {
			t.Errorf("%s figures are not published under a body policy", a)
		}
	}
}

// Writing the field out as false is how somebody turns the policy back off,
// and it has to read the same as never having written the file at all.
func TestAPolicyThatSaysFalsePublishesByLicence(t *testing.T) {
	c := policyCorpus(t, "body: false\nnote: turned back off while we ask the publishers\n")
	if c.PublishesWhole() {
		t.Error("body: false publishes everything")
	}
	if c.Publishes(AccessUnknown) {
		t.Error("an unresolved paper is published under body: false")
	}
}

// The nil corpus is what a caller that never opened one passes, and it should
// fall back to the licence rather than panic.
func TestTheNilCorpusPublishesByLicence(t *testing.T) {
	var c *Corpus
	if c.PublishesWhole() {
		t.Error("a nil corpus says it publishes everything")
	}
	if c.Publishes(AccessRestricted) {
		t.Error("a nil corpus publishes a restricted paper")
	}
	if !c.Publishes(AccessPublicDomain) {
		t.Error("a nil corpus does not publish a public domain paper")
	}
}

func TestAPolicyThatDoesNotParseIsAnError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"papers.yaml": threePapers,
		"policy.yaml": "body: [not a flag]\n",
	} {
		if err := os.WriteFile(filepath.Join(root, "manifests", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Open(root); err == nil {
		t.Fatal("a corpus with an unreadable policy opened anyway")
	}
}

func policyCorpus(t *testing.T, policy string) *Corpus {
	t.Helper()
	c := testCorpus(t, threePapers, "", testSources)
	if err := os.WriteFile(c.PolicyManifest(), []byte(policy), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Open(c.Root)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
