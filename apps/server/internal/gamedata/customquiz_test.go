package gamedata

import "testing"

func TestLoadCustomQuiz(t *testing.T) {
	q, err := LoadCustomQuiz(configDir())
	if err != nil {
		t.Fatal(err)
	}
	s := q.Slots[0]
	if s.Name != "Quiz" || s.AlarmTime != 3 || s.EventTime != 3 || len(s.Schedule) != 1 {
		t.Fatalf("slot 0 = %+v", s)
	}
	if sc := s.Schedule[0]; sc != (Schedule{-1, -1, -1, -1, 18, 15, 0}) {
		t.Fatalf("schedule %+v", sc)
	}
	if len(q.Questions) != 14 {
		t.Fatalf("%d questions", len(q.Questions))
	}
	if r := q.Questions[3]; r.Money != 3 || r.Answer != "Albert Einstein" {
		t.Fatalf("question 3 = %+v", r)
	}
	if len(q.Slots[1].Schedule) != 0 || q.Slots[1].Name != "" {
		t.Fatal("slot 1 should be empty")
	}
}
