package calc

import "testing"

func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Fatal("1 + 2 is not 3")
	}
}

func TestAddWrong(t *testing.T) {
	t.Log("adding")
	if got := Add(2, 2); got != 5 {
		t.Errorf("Add(2, 2) = %d, want 5", got)
	}
}
