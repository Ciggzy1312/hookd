package store

import (
	"errors"
	"net/http"
	"sync"
	"time"
	"uuid"
)

const DefaultMax = 500

var ErrNotFound = errors.New("inbox not found")

type Upstream struct {
	Status    int
	Body      []byte
	BodyTrunc bool
	Error     string
}

type Record struct {
	ID         string
	InboxID    string
	Method     string
	Path       string
	Query      string
	Headers    http.Header
	Body       []byte
	BodyTrunc  bool
	RemoteAddr string
	Duration   time.Duration
	ReceivedAt time.Time
	Replay     *Upstream
	Forward    *Upstream
}

type Store struct {
	mu      sync.Mutex
	max     int
	inboxes map[string]*inbox
	notify  func(Record)
}

type inbox struct {
	id      string
	records []Record
}

func New(max int) *Store {
	if max <= 0 {
		max = DefaultMax
	}
	return &Store{
		max:     max,
		inboxes: make(map[string]*inbox),
	}
}

func (s *Store) Create() string {
	id := uuid.NewV7().String()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inboxes[id] = &inbox{id: id}
	return id
}

func (s *Store) SetNotify(fn func(Record)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notify = fn
}

func (s *Store) Has(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.inboxes[id]
	return ok
}

func (s *Store) Append(inboxID string, rec Record) (Record, error) {
	if rec.ID == "" {
		rec.ID = uuid.NewV7().String()
	}
	rec.InboxID = inboxID
	if rec.ReceivedAt.IsZero() {
		rec.ReceivedAt = time.Now()
	}
	if rec.Headers != nil {
		rec.Headers = rec.Headers.Clone()
	}
	if rec.Body != nil {
		rec.Body = append([]byte(nil), rec.Body...)
	}

	s.mu.Lock()
	in, ok := s.inboxes[inboxID]
	if !ok {
		s.mu.Unlock()
		return Record{}, ErrNotFound
	}
	in.records = append(in.records, rec)
	if extra := len(in.records) - s.max; extra > 0 {
		in.records = in.records[extra:]
	}
	notify := s.notify
	s.mu.Unlock()
	if notify != nil {
		notify(rec)
	}
	return rec, nil
}

func (s *Store) List(inboxID string) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	in, ok := s.inboxes[inboxID]
	if !ok {
		return nil, ErrNotFound
	}
	out := make([]Record, len(in.records))
	copy(out, in.records)
	return out, nil
}

func (s *Store) Get(inboxID, recID string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	in, ok := s.inboxes[inboxID]
	if !ok {
		return Record{}, ErrNotFound
	}
	for i := range in.records {
		if in.records[i].ID == recID {
			return in.records[i], nil
		}
	}
	return Record{}, ErrNotFound
}

func (s *Store) SetReplay(inboxID, recID string, up Upstream) (Record, error) {
	return s.setHop(inboxID, recID, up, func(rec *Record, hop *Upstream) { rec.Replay = hop })
}

func (s *Store) SetForward(inboxID, recID string, up Upstream) (Record, error) {
	return s.setHop(inboxID, recID, up, func(rec *Record, hop *Upstream) { rec.Forward = hop })
}

func (s *Store) setHop(inboxID, recID string, up Upstream, apply func(*Record, *Upstream)) (Record, error) {
	if up.Body != nil {
		up.Body = append([]byte(nil), up.Body...)
	}
	s.mu.Lock()
	in, ok := s.inboxes[inboxID]
	if !ok {
		s.mu.Unlock()
		return Record{}, ErrNotFound
	}
	for i := range in.records {
		if in.records[i].ID != recID {
			continue
		}
		cp := up
		apply(&in.records[i], &cp)
		rec := in.records[i]
		notify := s.notify
		s.mu.Unlock()
		if notify != nil {
			notify(rec)
		}
		return rec, nil
	}
	s.mu.Unlock()
	return Record{}, ErrNotFound
}
