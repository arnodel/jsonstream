package jsonpath

import "testing"

func TestDecision(t *testing.T) {
	type testData struct {
		name     string
		decision Decision
		isYes    bool
		isNo     bool
	}
	tests := []testData{
		{
			name:     "DontKnow",
			decision: DontKnow,
			isYes:    false,
			isNo:     false,
		},
		{
			name:     "Yes",
			decision: Yes,
			isYes:    true,
			isNo:     false,
		},
		{
			name:     "No",
			decision: No,
			isYes:    false,
			isNo:     true,
		},
		{
			name:     "DontKnow | NoMoreAfter",
			decision: DontKnow | NoMoreAfter,
			isYes:    false,
			isNo:     false,
		},
		{
			name:     "Yes | NoMoreAfter",
			decision: Yes | NoMoreAfter,
			isYes:    true,
			isNo:     false,
		},
		{
			name:     "No | NoMoreAfter",
			decision: No | NoMoreAfter,
			isYes:    false,
			isNo:     true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			isYes := test.decision.IsYes()
			if isYes != test.isYes {
				t.Errorf("expected IsYes()=%t, got %t", test.isYes, isYes)
			}
			isNo := test.decision.IsNo()
			if isNo != test.isNo {
				t.Errorf("expected IsNo()=%t, got %t", test.isNo, isNo)
			}
		})
	}
}
