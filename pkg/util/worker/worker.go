package worker

import (
	"github.com/rs/xid"
	"github.com/sirupsen/logrus"
	"sync"
)

type Dispatcher struct {
	buffer  int
	mu      *sync.Mutex
	done    chan struct{}
	workers map[string]chan Work
	log     *logrus.Entry
}

func NewDispatcher(log *logrus.Entry, buffer int) Dispatcher {
	return Dispatcher{
		log:     log.WithField("cmp", "dispatcher"),
		buffer:  buffer,
		mu:      new(sync.Mutex),
		done:    make(chan struct{}),
		workers: map[string]chan Work{},
	}
}

func (d Dispatcher) Dispatch(key string, work Work) {
	d.mu.Lock()
	defer d.mu.Unlock()
	log := d.log.WithField("key", key)
	log.Infof("Dispatching work item.")

	workCh, ok := d.workers[key]
	if !ok {
		workCh = make(chan Work, d.buffer)
		go func() {
			for w := range workCh {
				id := xid.New().String()
				log.Infof("Running work item with id %q.", id)
				w()
				log.Infof("Completed work item with id %q.", id)
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

func (d Dispatcher) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()

	close(d.done)
	for _, workCh := range d.workers {
		close(workCh)
	}
}

type Work func()
