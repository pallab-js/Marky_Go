package watcher

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Event struct {
	Path  string
	Cause string
}

type Service struct {
	mu       sync.Mutex
	watcher  *fsnotify.Watcher
	root     string
	stop     chan struct{}
	done     chan struct{}
	recent   map[string]time.Time
	onChange func(Event)
}

func NewService(onChange func(Event)) *Service {
	return &Service{onChange: onChange, recent: map[string]time.Time{}}
}

func (s *Service) MarkSelfWrite(absPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recent[filepath.Clean(absPath)] = time.Now().Add(2 * time.Second)
}

func (s *Service) Start(root string) error {
	s.Stop()
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.watcher = w
	s.root = root
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	s.mu.Unlock()
	if err := s.addRecursive(root); err != nil {
		_ = w.Close()
		return err
	}
	go s.loop()
	return nil
}

func (s *Service) Stop() {
	s.mu.Lock()
	if s.stop != nil {
		close(s.stop)
		stop, done, w := s.stop, s.done, s.watcher
		s.stop, s.done, s.watcher = nil, nil, nil
		s.mu.Unlock()
		if w != nil {
			_ = w.Close()
		}
		<-done
		_ = stop
		return
	}
	s.mu.Unlock()
}

func (s *Service) addRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			_ = s.watcher.Add(path)
		}
		return nil
	})
}

func (s *Service) isSelfWrite(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	clean := filepath.Clean(path)
	for p, until := range s.recent {
		if now.After(until) {
			delete(s.recent, p)
		}
	}
	until, ok := s.recent[clean]
	return ok && now.Before(until)
}

func (s *Service) loop() {
	defer close(s.done)
	debounce := 450 * time.Millisecond
	timer := time.NewTimer(debounce)
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	pending := false
	lastPath := ""
	fire := func(cause string) {
		if !pending {
			return
		}
		pending = false
		if s.onChange != nil {
			s.onChange(Event{Path: lastPath, Cause: cause})
		}
	}
	for {
		s.mu.Lock()
		w, stop := s.watcher, s.stop
		s.mu.Unlock()
		if w == nil || stop == nil {
			return
		}
		select {
		case <-stop:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			_ = err
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			base := filepath.Base(ev.Name)
			if strings.HasPrefix(base, ".") || strings.Contains(filepath.ToSlash(ev.Name), "/.") {
				continue
			}
			if s.isSelfWrite(ev.Name) {
				continue
			}
			// Watch newly created directories.
			if ev.Op&(fsnotify.Create|fsnotify.Rename) != 0 {
				if st, err := os.Lstat(ev.Name); err == nil && st.IsDir() && st.Mode()&os.ModeSymlink == 0 {
					_ = s.addRecursive(ev.Name)
				}
			}
			// MVP correctness: coalesce and let app rebuild/refresh from source of truth.
			pending = true
			lastPath = ev.Name
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(debounce)

		case <-timer.C:
			fire("fsnotify")
		}
	}
}
