package gamedata

import (
	"fmt"
	"path/filepath"
)

// CustomQuiz is Data/Custom/CustomQuiz.txt (CCustomQuiz::Load,
// FUN_00456d30): section 0 Index + start schedule, section 1 Index Name
// AlarmTime EventTime (minutes), section 2 the questions ID Money
// "Question" "Answer". Up to 50 quiz slots.
type CustomQuiz struct {
	Slots     [CustomQuizSlots]CustomQuizSlot
	Questions map[int]QuizQuestion // std::map keyed by ID (first row wins)
}

// CustomQuizSlots is the size of the slot array (0x32 entries of 0x54).
const CustomQuizSlots = 50

// CustomQuizSlot is one slot: +0x1c Name[32], +0x3c AlarmTime, +0x40
// EventTime and the schedule vector at +0x44.
type CustomQuizSlot struct {
	Name                 string
	AlarmTime, EventTime int
	Schedule             []Schedule
}

// QuizQuestion is the 0x108-byte question record.
type QuizQuestion struct {
	ID, Money        int
	Question, Answer string
}

// LoadCustomQuiz reads Data/Custom/CustomQuiz.txt.
func LoadCustomQuiz(dataDir string) (*CustomQuiz, error) {
	secs, err := loadSections(filepath.Join(dataDir, "Custom", "CustomQuiz.json"))
	if err != nil {
		return nil, err
	}
	q := &CustomQuiz{Questions: map[int]QuizQuestion{}}
	slot := func(i, line int) (*CustomQuizSlot, error) {
		if i < 0 || i >= CustomQuizSlots {
			return nil, fmt.Errorf("CustomQuiz.txt line %d: index %d", line, i)
		}
		return &q.Slots[i], nil
	}
	for _, s := range secs {
		for _, l := range s.Rows {
			switch s.Index {
			case 0:
				v, err := rowInts(l, 8)
				if err != nil {
					return nil, err
				}
				sl, err := slot(v[0], l.No)
				if err != nil {
					return nil, err
				}
				sl.Schedule = append(sl.Schedule, Schedule{v[1], v[2], v[3], v[4], v[5], v[6], v[7]})
			case 1:
				if len(l.Tokens) < 4 {
					return nil, fmt.Errorf("CustomQuiz.txt line %d: %d columns", l.No, len(l.Tokens))
				}
				i, err := l.Int(0)
				if err != nil {
					return nil, err
				}
				sl, err := slot(i, l.No)
				if err != nil {
					return nil, err
				}
				if sl.AlarmTime, err = l.Int(2); err != nil {
					return nil, err
				}
				if sl.EventTime, err = l.Int(3); err != nil {
					return nil, err
				}
				sl.Name = cutString(l.Tokens[1], 0x1F)
			case 2:
				if len(l.Tokens) < 4 {
					return nil, fmt.Errorf("CustomQuiz.txt line %d: %d columns", l.No, len(l.Tokens))
				}
				id, err := l.Int(0)
				if err != nil {
					return nil, err
				}
				money, err := l.Int(1)
				if err != nil {
					return nil, err
				}
				if _, dup := q.Questions[id]; dup {
					continue
				}
				q.Questions[id] = QuizQuestion{ID: id, Money: money,
					Question: cutString(l.Tokens[2], 0x7F), Answer: cutString(l.Tokens[3], 0x7F)}
			}
		}
	}
	return q, nil
}

// cutString is strcpy_s into a buffer of n+1 bytes.
func cutString(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
