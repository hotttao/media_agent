package config

import (
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// fileSource 是文件配置源。
//
// - Get 读文件 + expandEnv + yaml unmarshal → map[string]any
// - Watch 通过 fsnotify 监听文件变更，200ms 去抖
// - Priority = 10（基线，最低优先级）
//
// NewLoader 内部自动创建 fileSource，外部不感知。
type fileSource struct {
	path    string
	watcher *fsnotify.Watcher
	stopCh  chan struct{}
	stopOnce sync.Once
	stopped  bool
}

// newFileSource 创建文件源（不启动 watch，watch 由 Loader.Watch 统一启动）。
func newFileSource(path string) (*fileSource, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := fw.Add(path); err != nil {
		_ = fw.Close()
		return nil, err
	}
	return &fileSource{
		path:    path,
		watcher: fw,
		stopCh:  make(chan struct{}),
	}, nil
}

// Get 读文件 + expandEnv + yaml unmarshal。
func (s *fileSource) Get() (map[string]any, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	expanded := expandEnv(string(raw))
	tree := map[string]any{}
	if err := yaml.Unmarshal([]byte(expanded), &tree); err != nil {
		return nil, err
	}
	return tree, nil
}

// Watch 启动 fsnotify 监听，文件变更时调 onChange。
func (s *fileSource) Watch(onChange func()) error {
	go s.loop(onChange)
	return nil
}

func (s *fileSource) loop(onChange func()) {
	var (
		lastFire time.Time
		debounce = 200 * time.Millisecond
	)
	for {
		select {
		case <-s.stopCh:
			return
		case event, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			if time.Since(lastFire) < debounce {
				continue
			}
			lastFire = time.Now()
			onChange()
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			_ = err
		}
	}
}

func (s *fileSource) Close() error {
	s.stopOnce.Do(func() {
		s.stopped = true
		close(s.stopCh)
	})
	return s.watcher.Close()
}

func (s *fileSource) Priority() int { return 10 }
