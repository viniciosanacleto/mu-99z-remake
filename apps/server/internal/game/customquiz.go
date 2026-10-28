package game

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// CustomQuiz (CCustomQuiz, 0x456a60..0x457d90; Custom/CustomQuiz.txt,
// CustomQuizSwitch in Event.dat). Each of the 50 slots alternates EMPTY
// (waiting for the schedule, with AlarmTime minutes of warnings) and
// START (one random question asked every minute for EventTime minutes).
// The first correct /resp wins the question's zen, the item bag 0x18 and
// a firework. Only one question is active at a time (CCustomQuiz+0x1080).

const (
	quizBlank = iota
	quizEmpty
	quizStart

	quizMsgAlarm    = 0x44E // "[%s] Começa em %d minuto(s)!"
	quizMsgQuestion = 0x44F // "[%s] Pergunta: (Use /Resp para responder)"
	quizMsgText     = 0x450 // "[%s] %s"
	quizMsgNoWinner = 0x451
	quizMsgWinner   = 0x452
	quizMsgAnswer   = 0x453
	quizMsgWrong    = 0x454
	quizBag         = 0x18
)

type quizSlot struct {
	Index    int
	data     *gamedata.CustomQuizSlot
	State    int       // +4
	Remain   int       // +8
	Target   time.Time // +0xc
	Tick     time.Time // +0x10
	AlarmMin int       // +0x14
	LastMin  int       // +0x18
}

type customQuiz struct {
	data     *gamedata.CustomQuiz
	slots    [gamedata.CustomQuizSlots]*quizSlot
	Active   bool // +0x1080
	Question int  // +0x1084
	Slot     int  // +0x1088
}

func (w *World) quizOn() bool { return w.cfg.Int("Event", "CustomQuizSwitch", 0) != 0 }

// loadQuiz is FUN_00456d30 plus the start-up part of FUN_004baa20.
func (w *World) loadQuiz() error {
	d, err := gamedata.LoadCustomQuiz(w.configDir)
	if err != nil {
		return err
	}
	q := &customQuiz{data: d, Question: -1, Slot: -1}
	now := w.now()
	for i := range q.slots {
		s := &quizSlot{Index: i, data: &d.Slots[i], Tick: now, AlarmMin: -1, LastMin: -1}
		q.slots[i] = s
		if w.quizOn() {
			s.State = quizEmpty
			w.quizSchedule(s)
		}
	}
	w.cmdWorld().quiz = q
	return nil
}

func registerCustomquiz(h *hooks) {
	h.tick1000 = append(h.tick1000, (*World).quizTick)
}

// quizNotice is FUN_004c0ab0 with a Message.txt text: a golden notice to
// every player.
func (w *World) quizNotice(n int, args ...any) {
	text := w.messageFor(nil, n)
	if len(args) > 0 {
		text = fmt.Sprintf(cFormat(text), args...)
	}
	w.noticeAll(NoticeGold, text)
}

// quizSchedule is FUN_00457760: the next start, or state BLANK
// (FUN_004576f0(0)) without one.
func (w *World) quizSchedule(s *quizSlot) {
	now := w.now()
	next, ok := nextSchedule(s.data.Schedule, now)
	if !ok {
		s.State = quizBlank
		return
	}
	s.Target = next
	s.Remain = int(math.Floor(next.Sub(now).Seconds()))
}

// quizTick is FUN_004571a0: every slot, once per second.
func (w *World) quizTick() {
	if w.cmd == nil || w.cmd.quiz == nil {
		return
	}
	q := w.cmd.quiz
	now := w.now()
	for _, s := range q.slots {
		if now.Sub(s.Tick) < time.Second {
			continue
		}
		s.Tick = now
		s.Remain = int(math.Floor(s.Target.Sub(now).Seconds()))
		switch s.State {
		case quizEmpty:
			w.quizEmptyProc(q, s)
		case quizStart:
			w.quizStartProc(q, s)
		}
	}
}

// quizEmptyProc is FUN_00457230: the minute warnings, then a random
// question (rand % count looked up by ID) and state START.
func (w *World) quizEmptyProc(q *customQuiz, s *quizSlot) {
	name := s.data.Name
	if s.Remain > 0 && s.Remain <= s.data.AlarmTime*60 {
		m := s.Remain / 60
		if s.Remain%60 == 0 {
			m--
		}
		s.AlarmMin = m
		if m != s.LastMin {
			s.LastMin = m
			w.quizNotice(quizMsgAlarm, name, m+1)
		}
	}
	if s.Remain >= 1 {
		return
	}
	id := -1
	if n := len(q.data.Questions); n > 0 {
		id = int(uint32(monsterRand()) % uint32(n))
	}
	if qq, ok := q.data.Questions[id]; ok {
		w.quizNotice(quizMsgQuestion, name)
		w.quizNotice(quizMsgText, name, qq.Question)
		q.Active, q.Question, q.Slot = true, id, s.Index
	} else {
		q.Active, q.Question, q.Slot = false, -1, -1
	}
	s.State = quizStart
	s.AlarmMin, s.LastMin = -1, -1
	s.Remain = s.data.EventTime * 60
	s.Target = w.now().Add(time.Duration(s.Remain) * time.Second)
}

// quizStartProc is FUN_004574f0: back to EMPTY once answered; the question
// again at every full minute; "no winner" at the end.
func (w *World) quizStartProc(q *customQuiz, s *quizSlot) {
	name := s.data.Name
	if !q.Active {
		q.Question, q.Slot = -1, -1
		s.State = quizEmpty
		s.AlarmMin, s.LastMin = -1, -1
		w.quizSchedule(s)
	}
	if s.Remain > 0 && s.Remain <= s.data.EventTime*60-1 && s.Remain%60 == 0 {
		if qq, ok := q.data.Questions[q.Question]; ok && q.Question >= 0 {
			w.quizNotice(quizMsgQuestion, name)
			w.quizNotice(quizMsgText, name, qq.Question)
		}
	}
	if s.Remain < 1 {
		w.quizNotice(quizMsgNoWinner, name)
		q.Active, q.Question, q.Slot = false, -1, -1
		s.State = quizEmpty
		s.AlarmMin, s.LastMin = -1, -1
		w.quizSchedule(s)
	}
}

// commandQuizAnswer is /resp (case 28: FUN_00434af0 + FUN_004578d0). With
// an active question, the whole argument is compared with the answer in
// lower case: a wrong answer gets 0x454 (to the player); the right one
// announces the winner (0x452) and the lower-case answer (0x453) to
// everybody, adds the question's zen (a player with 0 zen keeps 0, the
// total is capped at 2,000,000,000 by FUN_0050cf10), drops the item bag
// 0x18, ends the question and sends the C1 07 F3 40 00 x y firework
// (FUN_004dcb60) to the player and its viewers. No delay or charge.
func (w *World) commandQuizAnswer(o *Object, c command, arg string) {
	if w.cmd == nil || w.cmd.quiz == nil {
		return
	}
	q := w.cmd.quiz
	if !q.Active {
		return
	}
	qq, ok := q.data.Questions[q.Question]
	if !ok || q.Slot < 0 || q.Slot >= len(q.slots) {
		return
	}
	name := q.slots[q.Slot].data.Name
	answer := strings.ToLower(qq.Answer)
	if strings.ToLower(arg) != answer {
		w.systemMessage(o, fmt.Sprintf(cFormat(w.messageFor(nil, quizMsgWrong)), name))
		return
	}
	ch := o.P.Char
	w.quizNotice(quizMsgWinner, name, ch.Name)
	w.quizNotice(quizMsgAnswer, name, answer)
	if ch.Money != 0 {
		if v := uint64(uint32(ch.Money)) + uint64(uint32(qq.Money)); v <= maxZen {
			ch.Money = int64(v)
		} else {
			ch.Money = maxZen
		}
	}
	w.sendMoney(o)
	if w.hooks.itemBag != nil {
		w.hooks.itemBag(w, o, quizBag, -1, o.Map, o.X, o.Y)
	}
	q.Active = false
	q.slots[q.Slot].Remain = 0
	fw := packetFirework(0, byte(o.X), byte(o.Y))
	w.send(o, fw)
	w.broadcast(o, fw)
}
