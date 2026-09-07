package attachments

import "testing"

func TestPathTraversalRejected(t *testing.T) {
	for _, h := range []string{"../etc/passwd", "abc", "ZZ", string([]byte{0})} {
		if err := ValidateHash(h); err == nil {
			t.Fatalf("expected reject %q", h)
		}
	}
	ok := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := ValidateHash(ok); err != nil {
		t.Fatal(err)
	}
}

func TestPutDedup(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: dir}
	h1, err := s.Put([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	h2, err := s.Put([]byte("hello"))
	if err != nil || h1 != h2 {
		t.Fatalf("%s %s %v", h1, h2, err)
	}
}
