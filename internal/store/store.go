package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"biztracker/internal/models"
)

var (
	ErrNotFound = errors.New("not found")
)

type Data struct {
	Tasks      []models.Task      `json:"tasks"`
	PriceItems []models.PriceItem `json:"priceItems"`
}

type Store struct {
	mu       sync.Mutex
	path     string
	data     Data
	taskSeq  int
}

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "data.json")
	s := &Store{path: path}
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, err
		}
	} else {
		s.data.PriceItems = DefaultPriceList()
		if err := s.persistLocked(); err != nil {
			return nil, err
		}
	}
	s.rebuildTaskSeq()
	return s, nil
}

func (s *Store) rebuildTaskSeq() {
	max := 0
	for _, t := range s.data.Tasks {
		n := parseNumericSuffix(t.ID)
		if n > max {
			max = n
		}
	}
	s.taskSeq = max
}

func parseNumericSuffix(id string) int {
	// 简单支持 AC0012 这类后缀数字
	n := 0
	started := false
	for _, r := range id {
		if r >= '0' && r <= '9' {
			started = true
			n = n*10 + int(r-'0')
		} else if started {
			break
		}
	}
	return n
}

func (s *Store) persistLocked() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0644)
}

func (s *Store) persist() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persistLocked()
}

// --- Tasks ---

func (s *Store) ListTasks() []models.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]models.Task, len(s.data.Tasks))
	copy(out, s.data.Tasks)
	return out
}

func (s *Store) nextTaskID(prefix string) string {
	s.taskSeq++
	if prefix == "" {
		prefix = "AC"
	}
	return prefix + fmt.Sprintf("%04d", s.taskSeq)
}

func (s *Store) CreateTask(t models.Task) models.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.ID == "" {
		t.ID = s.nextTaskID("AC")
	}
	if t.Status == "" {
		t.Status = models.StatusPending
	}
	s.data.Tasks = append(s.data.Tasks, t)
	_ = s.persistLocked()
	return t
}

func (s *Store) UpdateTask(id string, t models.Task) (models.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Tasks {
		if s.data.Tasks[i].ID == id {
			t.ID = id
			if t.Status == "" {
				t.Status = models.StatusPending
			}
			s.data.Tasks[i] = t
			_ = s.persistLocked()
			return t, nil
		}
	}
	return models.Task{}, ErrNotFound
}

func (s *Store) DeleteTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Tasks {
		if s.data.Tasks[i].ID == id {
			s.data.Tasks = append(s.data.Tasks[:i], s.data.Tasks[i+1:]...)
			_ = s.persistLocked()
			return nil
		}
	}
	return ErrNotFound
}

// --- Price list ---

func (s *Store) ListPrices() []models.PriceItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]models.PriceItem, len(s.data.PriceItems))
	copy(out, s.data.PriceItems)
	return out
}

func (s *Store) CreatePrice(p models.PriceItem) models.PriceItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == "" {
		p.ID = nextPriceID(s.data.PriceItems)
	}
	s.data.PriceItems = append(s.data.PriceItems, p)
	_ = s.persistLocked()
	return p
}

func (s *Store) UpdatePrice(id string, p models.PriceItem) (models.PriceItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.PriceItems {
		if s.data.PriceItems[i].ID == id {
			p.ID = id
			s.data.PriceItems[i] = p
			_ = s.persistLocked()
			return p, nil
		}
	}
	return models.PriceItem{}, ErrNotFound
}

func (s *Store) DeletePrice(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.PriceItems {
		if s.data.PriceItems[i].ID == id {
			s.data.PriceItems = append(s.data.PriceItems[:i], s.data.PriceItems[i+1:]...)
			_ = s.persistLocked()
			return nil
		}
	}
	return ErrNotFound
}

func nextPriceID(items []models.PriceItem) string {
	max := 0
	for _, it := range items {
		if strings.HasPrefix(it.ID, "P") && len(it.ID) > 1 {
			if n, err := strconv.Atoi(strings.TrimPrefix(it.ID, "P")); err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("P%04d", max+1)
}
