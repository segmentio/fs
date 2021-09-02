package fs

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

type watcher struct {
	wd   uint32
	path string
}

type handler struct {
	inotify  int
	watchers []watcher
	events   chan<- string
	done     chan struct{}
	lock     uint32
	once     sync.Once
	join     sync.WaitGroup
}

func (h *handler) wait() {
	h.join.Wait()
}

func (h *handler) close() {
	h.once.Do(func() {
		synchronized(func() {
			if handlers[h.events] == h {
				delete(handlers, h.events)
				delete(inotifys, h.inotify)
			}
		})
		if h.done != nil {
			close(h.done)
		}
		if h.inotify >= 0 {
			syscall.Close(h.inotify)
		}
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

func (h *handler) lookup(wd uint32) *watcher {
	i := sort.Search(len(h.watchers), func(i int) bool {
		return h.watchers[i].wd >= wd
	})
	if i < len(h.watchers) && h.watchers[i].wd == wd {
		return &h.watchers[i]
	}
	return nil
}

const (
	// struct inotify_event {
	//   int      wd;       /* Watch descriptor */
	//   uint32_t mask;     /* Mask describing event */
	//   uint32_t cookie;   /* Unique cookie associating related
	//                         events (for rename(2)) */
	//   uint32_t len;      /* Size of name field */
	//   char     name[];   /* Optional null-terminated name */
	// };
	sizeOfInotifyEvent = 16
)

var (
	mutex    sync.Mutex
	handlers map[chan<- string]*handler
	inotifys map[int]*handler
	epollfd  int
	epollerr error
)

func init() {
	handlers = make(map[chan<- string]*handler)
	inotifys = make(map[int]*handler)
	epollfd, epollerr = syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if epollerr == nil {
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
	buffer := [sizeOfInotifyEvent + syscall.NAME_MAX + 1]byte{}
	events := [64]syscall.EpollEvent{}

	for {
		n, err := syscall.EpollWait(epollfd, events[:], -1)
		if err != nil {
			if err != syscall.EINTR {
				fmt.Fprintf(os.Stderr, "[epoll] ERROR: %s\n", err)
				time.Sleep(1 * time.Second)
			}
			continue
		}

		synchronized(func() {
			for _, e := range events[:n] {
				h := inotifys[int(e.Fd)]
				if h == nil {
					continue
				}

			readInotifyEvent:
				n, err := syscall.Read(int(e.Fd), buffer[:])
				if err == syscall.EINTR {
					goto readInotifyEvent
				}
				if err == syscall.EAGAIN {
					continue
				}
				if err != nil {
					fmt.Fprintf(os.Stderr, "[inotify] ERROR: %s\n", err)
					continue
				}

				event, _, _ := read(buffer[:n])

				if w := h.lookup(uint32(event.Wd)); w != nil {
					h.notify(w.path)
				}

				for i := range buffer[:n] {
					buffer[i] = 0
				}
			}
		})
	}
}

func read(b []byte) (*syscall.InotifyEvent, []byte, []byte) {
	if len(b) < sizeOfInotifyEvent {
		return nil, nil, nil
	}
	event := (*syscall.InotifyEvent)(unsafe.Pointer(&b[0]))
	i := sizeOfInotifyEvent
	j := sizeOfInotifyEvent + int(event.Len)
	return event, trimNullBytes(b[i:j]), b[j:]
}

func trimNullBytes(b []byte) []byte {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return b
}

func notify(events chan<- string, paths []string) error {
	if epollerr != nil {
		return os.NewSyscallError("epoll_create1", epollerr)
	}

	inotify, err := syscall.InotifyInit1(syscall.IN_NONBLOCK | syscall.IN_CLOEXEC)
	if err != nil {
		return os.NewSyscallError("inotify_init1", err)
	}

	handler := &handler{
		inotify:  inotify,
		events:   events,
		watchers: make([]watcher, 0, len(paths)),
		done:     make(chan struct{}),
	}

	defer func() {
		if handler != nil {
			handler.close()
			handler.wait()
		}
	}()

	for _, path := range paths {
		wd, err := syscall.InotifyAddWatch(handler.inotify, path, 0|
			syscall.IN_ATTRIB|
			syscall.IN_CREATE|
			syscall.IN_DELETE|
			syscall.IN_DELETE_SELF|
			syscall.IN_MODIFY|
			syscall.IN_MOVE_SELF|
			syscall.IN_MOVED_FROM|
			syscall.IN_MOVED_TO|
			syscall.IN_DONT_FOLLOW|
			syscall.IN_EXCL_UNLINK,
		)
		if err != nil {
			return os.NewSyscallError("inotify_add_watch", err)
		}
		handler.watchers = append(handler.watchers, watcher{
			wd:   uint32(wd),
			path: path,
		})
	}

	sort.Slice(handler.watchers, func(i, j int) bool {
		return handler.watchers[i].wd < handler.watchers[j].wd
	})

	synchronized(func() {
		if err = syscall.EpollCtl(epollfd, syscall.EPOLL_CTL_ADD, handler.inotify, &syscall.EpollEvent{
			Events: syscall.EPOLLIN | syscall.EPOLLONESHOT,
			Fd:     int32(handler.inotify),
		}); err != nil {
			return
		}
		inotifys[handler.inotify] = handler
		handlers[events], handler = handler, handlers[events]
	})

	if err != nil {
		return os.NewSyscallError("epoll_ctl", err)
	}

	return nil
}

func stop(events chan<- string) {
	var handler *handler
	synchronized(func() {
		if handler = handlers[events]; handler != nil {
			delete(handlers, events)
			delete(inotifys, handler.inotify)
		}
	})
	if handler != nil {
		handler.close()
		handler.wait()
	}
}
