package fs

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type file struct {
	fd   int
	path string
}

type handler struct {
	files  []file
	events chan<- string
	done   chan struct{}
	lock   uint32
	once   sync.Once
	join   sync.WaitGroup
}

func (h *handler) wait() {
	h.join.Wait()
}

func (h *handler) close() {
	h.once.Do(func() {
		synchronized(func() {
			if handlers[h.events] == h {
				delete(handlers, h.events)
			}
		})
		if h.done != nil {
			close(h.done)
		}
		for _, f := range h.files {
			syscall.Close(f.fd)
		}
		h.files = nil
	})
}

func (h *handler) notify(path string) {
	if atomic.CompareAndSwapUint32(&h.lock, 0, 1) {
		h.join.Add(1)
		go func() {
			defer h.join.Done()
			defer h.close()
			select {
			case h.events <- path:
			case <-h.done:
			}
		}()
	}
}

func (h *handler) lookup(fd int) *file {
	i := sort.Search(len(h.files), func(i int) bool {
		return h.files[i].fd >= fd
	})
	if i < len(h.files) && h.files[i].fd == fd {
		return &h.files[i]
	}
	return nil
}

var (
	mutex    sync.Mutex
	handlers map[chan<- string]*handler
	kqueue   int
	kerr     error
)

func init() {
	handlers = make(map[chan<- string]*handler)
	kqueue, kerr = syscall.Kqueue()
	if kerr == nil {
		go run()
	}
}

func synchronized(f func()) {
	mutex.Lock()
	defer mutex.Unlock()
	f()
}

func run() {
	runtime.LockOSThread()
	events := make([]syscall.Kevent_t, 64)

	for {
		n, err := syscall.Kevent(kqueue, nil, events, nil)
		if err != nil {
			if err != syscall.EINTR {
				fmt.Fprintf(os.Stderr, "[kevent] ERROR: %s\n", err)
				time.Sleep(1 * time.Second)
			}
			continue
		}

		synchronized(func() {
			for _, e := range events[:n] {
				if e.Filter != syscall.EVFILT_VNODE {
					continue
				}

				/*
					| Condition                               | Events                   |
					| --------------------------------------- | ------------------------ |
					| creating a file in a directory          | NOTE_WRITE               |
					| creating a directory in a directory     | NOTE_WRITE NOTE_LINK     |
					| creating a link in a directory          | NOTE_WRITE               |
					| creating a symlink in a directory       | NOTE_WRITE               |
					| removing a file from a directory        | NOTE_WRITE               |
					| removing a directory from a directory   | NOTE_WRITE NOTE_LINK     |
					| renaming a file within a directory      | NOTE_WRITE               |
					| renaming a directory within a directory | NOTE_WRITE               |
					| moving a file out of a directory        | NOTE_WRITE               |
					| moving a directory out of a directory   | NOTE_WRITE NOTE_LINK     |
					| writing to a file                       | NOTE_WRITE NOTE_EXTEND   |
					| truncating a file                       | NOTE_ATTRIB              |
					| overwriting a symlink                   | NOTE_DELETE, NOTE_RENAME |
				*/

				for _, h := range handlers {
					if f := h.lookup(int(e.Ident)); f != nil {
						h.notify(f.path)
					}
				}
			}
		})
	}
}

func notify(events chan<- string, paths []string) error {
	if kerr != nil {
		return os.NewSyscallError("kqueue", kerr)
	}

	changes := make([]syscall.Kevent_t, len(paths))
	handler := &handler{
		files:  make([]file, 0, len(paths)),
		done:   make(chan struct{}),
		events: events,
	}

	defer func() {
		if handler != nil {
			handler.close()
		}
	}()

	for _, path := range paths {
		fd, err := syscall.Open(path, syscall.O_SYMLINK|syscall.O_EVTONLY|syscall.O_CLOEXEC, 0)
		if err != nil {
			return &os.PathError{
				Op:   "open",
				Path: path,
				Err:  err,
			}
		}
		handler.files = append(handler.files, file{
			fd:   fd,
			path: path,
		})
	}

	for i, f := range handler.files {
		changes[i] = syscall.Kevent_t{
			Ident:  uint64(f.fd),
			Filter: syscall.EVFILT_VNODE,
			Flags:  syscall.EV_ADD | syscall.EV_CLEAR | syscall.EV_ONESHOT,
			Fflags: syscall.NOTE_DELETE |
				syscall.NOTE_WRITE |
				syscall.NOTE_EXTEND |
				syscall.NOTE_ATTRIB |
				syscall.NOTE_LINK |
				syscall.NOTE_RENAME,
		}
	}

	if _, err := syscall.Kevent(kqueue, changes, nil, nil); err != nil {
		return os.NewSyscallError("kevent", err)
	}

	sort.Slice(handler.files, func(i, j int) bool {
		return handler.files[i].fd < handler.files[j].fd
	})

	synchronized(func() {
		handlers[events], handler = handler, handlers[events]
	})

	return nil
}

func stop(events chan<- string) {
	var handler *handler
	synchronized(func() {
		handler = handlers[events]
		delete(handlers, events)
	})
	if handler != nil {
		handler.close()
		handler.wait()
	}
}
