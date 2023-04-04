package sharedcpuspool

import "testing"

func TestContainerIdent(t *testing.T) {
	ciA := ContainerIdent{}
	if !ciA.IsEmpty() {
		t.Fatalf("containerIdent not detected as empty")
	}

	ciA2 := ContainerIdent{
		Namespace: "xxx",
	}
	if !ciA2.IsEmpty() {
		t.Fatalf("containerIdent not detected as empty (with partial fill)")
	}

	ciB := ContainerIdent{
		Namespace:     "foo",
		PodName:       "bar",
		ContainerName: "baz",
	}
	if ciB.IsEmpty() {
		t.Fatalf("containerIdent misdetected as empty")
	}
	got := ciB.String()
	exp := "foo/bar/baz"
	if got != exp {
		t.Fatalf("string failed: got=%q expected=%q", got, exp)
	}

	expId := "ns/pn/cn"
	ciC, err := ContainerIdentFromString(expId)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotId := ciC.String()
	if gotId != expId {
		t.Fatalf("fromstring RTT failed: got=%q expected=%q", gotId, expId)
	}

	if _, err := ContainerIdentFromString("zzzz"); err == nil {
		t.Fatalf("accepted malformed ident")
	}
}
