package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/githubflyideas/unagikuma/internal/store"
)

// Channels send only to people still in the directory: someone who left
// and an address that keeps bouncing are dropped; @handles expand.
func TestChannelsFollowTheDirectory(t *testing.T) {
	dir := t.TempDir()
	bind := `@ IN SOA t. admin.localhost. ( 1 1m 15s 5s 3 )
mail IN CHANNEL email host=127.0.0.1 port=25 tls=none from=mon@example.com to="li@example.com, wang@example.com, @noc"
sms  IN CHANNEL aliyun-sms access-key-id=a access-key-secret=b sign-name=c template-code=d phones=13800000002
`
	if err := os.WriteFile(filepath.Join(dir, "monitors.bind"), []byte(bind), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{DataDir: dir, Probe: (&fakeProbes{fail: map[string]bool{}}).run})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Store().Close()
	st := a.Store()
	for _, p := range []store.Person{
		{Handle: "li", Emails: []string{"li@example.com"}},
		{Handle: "wang", Emails: []string{"wang@example.com"}, Phones: []string{"+86 138 0000 0002"}},
		{Handle: "noc", Emails: []string{"noc@example.com"}},
	} {
		if _, err := st.SavePerson(p); err != nil {
			t.Fatal(err)
		}
	}
	a.DirectoryChanged()
	opts := func(ch string) map[string]string { return a.engineConfig(a.applied.Load()).Channels[ch].Opts }
	if got := opts("mail")["to"]; got != "li@example.com,wang@example.com,noc@example.com" {
		t.Fatalf("to before: %q", got)
	}
	people, _ := st.People()
	for _, p := range people {
		if p.Handle == "wang" {
			if err := st.SetLeft(p.ID, true); err != nil {
				t.Fatal(err)
			}
		}
	}
	for range 3 {
		if err := st.RecordBounce(store.ContactEmail, "NOC@example.com", "550 5.1.1 user unknown"); err != nil {
			t.Fatal(err)
		}
	}
	a.DirectoryChanged()
	if got := opts("mail")["to"]; got != "li@example.com" {
		t.Fatalf("to after: %q", got)
	}
	ec := a.engineConfig(a.applied.Load())
	if ec.Channels["sms"].NoRecipients == "" {
		t.Fatalf("sms to someone who left: %+v", ec.Channels["sms"])
	}
	issues := a.ChannelIssues()
	if len(issues["mail"]) != 2 || len(issues["sms"]) != 2 {
		t.Fatalf("issues: %+v", issues)
	}
}
