package telegram

import (
	"errors"
	"sync"

	"PicFolderBot/internal/service"
)

type uploadResult struct {
	Target string
	Err    error
}

type uploadTask struct {
	Level   string
	Payload service.UploadPayload
	Done    chan uploadResult
}

type uploader struct {
	flow    flowAPI
	workers int
	queue   chan uploadTask

	mu     sync.RWMutex
	closed bool
	once   sync.Once
	wg     sync.WaitGroup
}

func newUploader(flow flowAPI, workers int, queueSize int) *uploader {
	if workers <= 0 {
		workers = 3
	}
	if queueSize <= 0 {
		queueSize = 128
	}
	u := &uploader{
		flow:    flow,
		workers: workers,
		queue:   make(chan uploadTask, queueSize),
	}
	u.start()
	return u
}

func (u *uploader) start() {
	u.once.Do(func() {
		for i := 0; i < u.workers; i++ {
			u.wg.Add(1)
			go func() {
				defer u.wg.Done()
				for task := range u.queue {
					target, err := u.flow.UploadImageAtLevel(task.Level, task.Payload)
					task.Done <- uploadResult{Target: target, Err: err}
					close(task.Done)
				}
			}()
		}
	})
}

func (u *uploader) stop() {
	u.mu.Lock()
	if u.closed {
		u.mu.Unlock()
		return
	}
	u.closed = true
	u.mu.Unlock()
	close(u.queue)
	u.wg.Wait()
}

func (u *uploader) submit(level string, payload service.UploadPayload) <-chan uploadResult {
	done := make(chan uploadResult, 1)
	u.mu.RLock()
	closed := u.closed
	u.mu.RUnlock()
	if closed {
		done <- uploadResult{Err: errors.New("uploader is stopped")}
		close(done)
		return done
	}
	u.queue <- uploadTask{Level: level, Payload: payload, Done: done}
	return done
}
