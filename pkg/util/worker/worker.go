package worker

import (
	"github.com/sirupsen/logrus"
	"sync"
)

type KeyedWorkQueue struct {
	buffer  int
	mu      *sync.Mutex
	done    chan struct{}
	workers map[string]chan Work
	log     *logrus.Entry
	wg 		*sync.WaitGroup
}

func NewKeyedWorkQueue(log *logrus.Entry, buffer int) KeyedWorkQueue {
	return KeyedWorkQueue{
		log:     log.WithField("cmp", "dispatcher"),
		buffer:  buffer,
		mu:      new(sync.Mutex),
		done:    make(chan struct{}),
		wg:      new(sync.WaitGroup),
		workers: map[string]chan Work{},
	}
}

func (d KeyedWorkQueue) Dispatch(key string, work Work) {
	d.wg.Add(1)
	d.mu.Lock()
	defer d.mu.Unlock()
	// log := d.log.WithField("key", key)
	// log.Infof("Dispatching work item.")

	workCh, ok := d.workers[key]
	if !ok {
		workCh = make(chan Work, d.buffer)
		go func() {
			for w := range workCh {
				// id := xid.New().String()
				// log.Infof("Running work item with id %q.", id)
				w()
				d.wg.Done()
				// log.Infof("Completed work item with id %q.", id)
			}
		}()
		d.workers[key] = workCh
	}

	select {
	case <-d.done:
		return
	case workCh <- work:
	}
}

func (d KeyedWorkQueue) Wait() {
	d.wg.Wait()
}

func (d KeyedWorkQueue) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()

	close(d.done)
	for _, workCh := range d.workers {
		close(workCh)
	}
}

type Work func()
