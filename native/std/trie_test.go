package std

import "testing"

func TestTrieInsertFindRemove(t *testing.T) {
	n := NewNode()
	n.Insert("cat", "car", "card")

	for _, w := range []string{"cat", "car", "card"} {
		if !n.Find(w) {
			t.Errorf("Find(%q) = false, want true", w)
		}
	}
	if n.Find("ca") { // prefix, not a complete word
		t.Errorf("Find(%q) = true, want false (prefix is not a leaf)", "ca")
	}
	if n.Find("cards") {
		t.Errorf("Find(%q) = true, want false", "cards")
	}
	if got := n.Size(); got != 3 {
		t.Errorf("Size() = %d, want 3", got)
	}

	n.Remove("car")
	if n.Find("car") {
		t.Errorf("Find(%q) after Remove = true, want false", "car")
	}
	// "card" shares the "car" prefix and must remain reachable after the
	// lazy removal of "car".
	if !n.Find("card") {
		t.Errorf("Find(%q) after removing %q = false, want true", "card", "car")
	}
	if got := n.Size(); got != 2 {
		t.Errorf("Size() after Remove = %d, want 2", got)
	}
}
